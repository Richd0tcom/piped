package maestro

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"sync"
	"time"

	"github.com/richd0tcom/piped/internal/filemanager"
	"github.com/richd0tcom/piped/internal/models"
	"github.com/richd0tcom/piped/internal/portal"
	"github.com/richd0tcom/piped/internal/proxy"
	"github.com/richd0tcom/piped/internal/store"
	"github.com/richd0tcom/piped/internal/vessel"
)

type Maestro struct {
	store  *store.Store
	portal *portal.Portal
	vessel *vessel.Vessel
	proxy  *proxy.Proxy
	fm     *filemanager.FileManager

	runningLogCancels sync.Map // deploymentID -> context.CancelFunc
}

func New(s *store.Store, p *portal.Portal, v *vessel.Vessel, px *proxy.Proxy, fm *filemanager.FileManager) *Maestro {
	return &Maestro{store: s, portal: p, vessel: v, proxy: px, fm: fm}
}


func (m *Maestro) Deploy(deploymentID string) {
	go m.run(deploymentID)
}

func (m *Maestro) run(deploymentID string) {
	fmt.Println("running deployment", deploymentID)
	ctx := context.Background()

	d, err := m.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		fmt.Println("there was an error getting deployment", err)
		return
	}

	m.log(d.ID, "system", "Pipeline started")

	
	srcDir, err := m.fm.TempDir()
	if err != nil {
		m.fail(d, fmt.Sprintf("temp dir: %v", err))
		return
	}
	defer m.fm.Cleanup(srcDir)

	err = m.withRetry(3, "prepare source", func() error {
		if d.SourceType == models.SourceGit {
			//TODO: enable option for deploying specific commits
			return m.fm.CloneRepo(ctx, d.ResourceURL,  srcDir)
		}
		return m.fm.ExtractArchive(d.ResourceURL, srcDir) 
	})
	if err != nil {
		m.fail(d, fmt.Sprintf("prepare: %v", err))
		return
	}

	//build image
	m.setStatus(d, models.StatusBuilding)
	imageTag := fmt.Sprintf("piped/%s:%d", d.ID[:8], time.Now().Unix())

	logWriter := m.newLogWriter(d.ID, "stdout")
	err = m.withRetry(3, "build image", func() error {
		return m.vessel.BuildImage(ctx, srcDir, imageTag, d.EnvVars, logWriter)
	})
	logWriter.Close()
	if err != nil {
		m.fail(d, fmt.Sprintf("build: %v", err))
		return
	}

	d.ImageTag = &imageTag
	m.store.UpdateDeployment(ctx, d)
	m.log(d.ID, "system", fmt.Sprintf("Image built: %s", imageTag))

	appPort, err := m.vessel.GetImagePort(ctx, imageTag)
	if err != nil {
		m.fail(d, fmt.Sprintf("get image port: %v", err))
		return
	}

	var containerName string


	// deploy (blue-green)
	m.setStatus(d, models.StatusDeploying)

	//TODO: fix potential port collision on freeport()
	// port, err := freePort()
	// if err != nil {
	// 	m.fail(d, fmt.Sprintf("find port: %v", err))
	// 	return
	// }



	var standbyID string
	err = m.withRetry(3, "start container", func() error {

		containerName = fmt.Sprintf("deploy-%s-%d", d.ID[:8], time.Now().UnixNano())

		id, err := m.vessel.RunContainer(ctx, imageTag, containerName, d.EnvVars, appPort)
		standbyID = id
		return err
	})
	if err != nil {
		m.fail(d, fmt.Sprintf("run container: %v", err))
		return
	}

	d.StandbyContainerID = &standbyID
	m.store.UpdateDeployment(ctx, d)

	deployCtx, cancelDeployLogs := context.WithCancel(ctx)
	go func() {
		stdout := m.newLogWriter(d.ID, "stdout")
		stderr := m.newLogWriter(d.ID, "stderr")
		defer stdout.Close()
		defer stderr.Close()
		m.vessel.StreamContainerLogs(deployCtx, standbyID, stdout, stderr)
	}()

	if err := m.vessel.WaitForHealthy(ctx, standbyID, 30*time.Second); err != nil {
		cancelDeployLogs()
		m.fail(d, fmt.Sprintf("health check: %v", err))
		return
	}

	cancelDeployLogs()

	//swap cady routes
	err = m.withRetry(3, "caddy route", func() error {
		if d.ActiveContainerID == nil {
			return m.proxy.AddRoute(d.ID, containerName, appPort)
		}
		return m.proxy.SwapRoute(d.ID, containerName, appPort)
	})
	if err != nil {
		m.fail(d, fmt.Sprintf("caddy: %v", err))
		return
	}

	// tear down old container
	if d.ActiveContainerID != nil {
		m.vessel.StopContainer(ctx, *d.ActiveContainerID)
		m.vessel.RemoveContainer(ctx, *d.ActiveContainerID)
	}

	d.ActiveContainerID = &standbyID
	d.StandbyContainerID = nil
	portPtr := appPort
	d.Port = &portPtr
	caddyRoute := fmt.Sprintf("/deploy/%s/", d.ID)
	d.CaddyRoute = &caddyRoute



	m.stopRunningLogs(d.ID) // cancel any previous instance (redeploy case)

	runCtx, cancelRunLogs := context.WithCancel(context.Background())
	m.runningLogCancels.Store(d.ID, cancelRunLogs)

	go func() {
		stdout := m.newLogWriter(d.ID, "stdout")
		stderr := m.newLogWriter(d.ID, "stderr")
		defer stdout.Close()
		defer stderr.Close()
		if err := m.vessel.StreamContainerLogs(runCtx, standbyID, stdout, stderr); err != nil {
			m.log(d.ID, "system", fmt.Sprintf("log stream ended: %v", err))
		}
	}()

	m.setStatus(d, models.StatusRunning)
	m.log(d.ID, "system", fmt.Sprintf("Running at %s", *d.CaddyRoute))
}

// Rollback redeploys a specific image tag without rebuilding.
func (m *Maestro) Rollback(deploymentID, imageTag string) error {

	//TODO: properly implement rollback and redeploy
	//Take note of m.stopRunningLogs(deploymentID)
	ctx := context.Background()
	d, err := m.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}

	// port, err := freePort()
	// if err != nil {
	// 	return err
	// }

	appPort, err := m.vessel.GetImagePort(ctx, imageTag)
	if err != nil {
		return err
	}

	standbyID, err := m.vessel.RunContainer(ctx, imageTag, fmt.Sprintf("%s", d.ID[:8]), d.EnvVars, appPort)
	if err != nil {
		return err
	}

	if err := m.vessel.WaitForHealthy(ctx, standbyID, 30*time.Second); err != nil {
		m.vessel.RemoveContainer(ctx, standbyID)
		return err
	}

	m.stopRunningLogs(d.ID)



	runCtx, cancelRunLogs := context.WithCancel(context.Background())
	m.runningLogCancels.Store(d.ID, cancelRunLogs)
	go func() {
		stdout := m.newLogWriter(d.ID, "stdout")
		stderr := m.newLogWriter(d.ID, "stderr")
		defer stdout.Close()
		defer stderr.Close()
		if err := m.vessel.StreamContainerLogs(runCtx, standbyID, stdout, stderr); err != nil {
			m.log(d.ID, "system", fmt.Sprintf("log stream ended: %v", err))
		}
	}()

	//should container name be unique?
	containerName := "deploy-" + d.ID[:8]

	if err := m.proxy.SwapRoute(d.ID, containerName, appPort); err != nil {
		m.vessel.StopContainer(ctx, standbyID)
		m.vessel.RemoveContainer(ctx, standbyID)

		m.stopRunningLogs(d.ID)
		return err
	}

	if d.ActiveContainerID != nil {
		m.vessel.StopContainer(ctx, *d.ActiveContainerID)
		m.vessel.RemoveContainer(ctx, *d.ActiveContainerID)
	}


	d.ActiveContainerID = &standbyID
	d.ImageTag = &imageTag
	portPtr := appPort
	d.Port = &portPtr
	d.Status = models.StatusRunning
	return m.store.UpdateDeployment(ctx, d)
}

// Restart stops and restarts the current container from the same image.
func (m *Maestro) Restart(deploymentID string) error {
	ctx := context.Background()
	
	d, err := m.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}
	return m.Rollback(deploymentID, *d.ImageTag)
}



func (m *Maestro) setStatus(d *models.Deployment, status models.DeploymentStatus) {

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    
    d.Status = status
    m.store.UpdateDeployment(ctx, d)
    
    // Publish status update to portal
    m.portal.PublishStatus(d.ID, string(status))
}

func (m *Maestro) fail(d *models.Deployment, reason string) {
	m.log(d.ID, "system", "FAILED: "+reason)

        
    go func() {
        m.setStatus(d, models.StatusFailed)
    }()
}

func (m *Maestro) log(deploymentID, stream, text string) {
	fmt.Println("deployemnt ID : ", deploymentID, " stream : ", stream, " text : ", text)
	m.portal.Publish(models.LogEntry{DeploymentID: deploymentID, Stream: stream, Text: text})
}

//an io.Writer that splits lines and publishes each to portal.
func (m *Maestro) newLogWriter(deploymentID, stream string) io.WriteCloser {
	pr, pw := io.Pipe()
	go func() {
		scanner := bufio.NewScanner(pr)
		for scanner.Scan() {
			m.log(deploymentID, stream, scanner.Text())
		}
	}()
	return pw
}

func (m *Maestro) withRetry(attempts int, label string, fn func() error) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		wait := time.Duration(math.Pow(2, float64(i))) * time.Second
		m.portal.Publish(models.LogEntry{
			Stream: "system",
			Text:   fmt.Sprintf("[retry %d/%d] %s: %v — retrying in %s", i+1, attempts, label, err, wait),
		})
		time.Sleep(wait)
	}
	return fmt.Errorf("%s failed after %d attempts: %w", label, attempts, err)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}


func (m *Maestro) Teardown(deploymentID string) error {
	ctx := context.Background()
	d, err := m.store.GetDeployment(ctx, deploymentID)
	if err != nil {
		return err
	}
	if d.ActiveContainerID != nil {
		m.vessel.StopContainer(ctx, *d.ActiveContainerID)
		m.vessel.RemoveContainer(ctx, *d.ActiveContainerID)
	}
	if d.StandbyContainerID != nil {
		m.vessel.StopContainer(ctx, *d.StandbyContainerID)
		m.vessel.RemoveContainer(ctx, *d.StandbyContainerID)
	}

	m.stopRunningLogs(deploymentID)

	m.proxy.RemoveRoute(deploymentID)
	d.Status = models.StatusDestroyed
	return m.store.UpdateDeployment(ctx, d)
}

func (m *Maestro) stopRunningLogs(deploymentID string) {
    if v, ok := m.runningLogCancels.LoadAndDelete(deploymentID); ok {
        v.(context.CancelFunc)()
    }
}