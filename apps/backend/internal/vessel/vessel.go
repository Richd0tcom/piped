package vessel

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/creack/pty"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

type ContainerInfo struct {
	ID     string
	Port   int
	Status string
}
 
type Vessel struct {
	docker *client.Client
}


func New() (*Vessel, error ){
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())

	if err != nil {
		return nil, err
	}

	return &Vessel{
		docker: cli,
	}, nil
}

func (v *Vessel) BuildImage(ctx context.Context, srcDir, tag string, envVars map[string]string, logWriter io.Writer) error {
	fmt.Println("Building image with tag:", tag)
	args := []string{"build", "--name", tag, srcDir}
	
	cmd :=  exec.CommandContext(ctx, "railpack", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter

	// passes env vars as build args via environment
	cmd.Env = os.Environ()

	cmd.Env = append(cmd.Env, "BUILDKIT_HOST=tcp://buildkitd:1234")

	for k, val := range envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, val))
	}
	ptmx, err := pty.Start(cmd)
    if err != nil {
        return fmt.Errorf("pty start: %w", err)
    }
    defer ptmx.Close()

    // PTY merges stdout+stderr into one stream, copy it to your log writer
    io.Copy(logWriter, ptmx)

    return cmd.Wait()
}

func (v *Vessel) RunContainer(ctx context.Context, image string, containerName string, envVars map[string]string, hostPort int) (string, error) {

	env := make([]string, 0, len(envVars))
	for k, val := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", k, val))
	}
 
	// portBinding := nat.PortMap{
	// 	"3000/tcp": []nat.PortBinding{{HostIP: "127.0.0.1", HostPort: fmt.Sprintf("%d", hostPort)}},
	// }

	resp, err := v.docker.ContainerCreate(ctx,
		&container.Config{
			Image: image,
			Env:   env,
		},
		&container.HostConfig{
			// PortBindings: portBinding,

			//TODO: configure this 
			RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
			NetworkMode:   "piped-network", //TODO: get this from env
		},
		nil, nil, containerName,
	)
	if err != nil {
		return "", fmt.Errorf("create container: %w", err)
	}
 
	if err := v.docker.ContainerStart(ctx, resp.ID, container.StartOptions{}); err != nil {
		return "", fmt.Errorf("start container: %w", err)
	}
	return resp.ID, nil
}

func (v *Vessel) StreamContainerLogs(ctx context.Context, containerID string, stdout, stderr io.Writer) error {

	fmt.Println("streaming container logs")
    out, err := v.docker.ContainerLogs(ctx, containerID, container.LogsOptions{
        ShowStdout: true,
        ShowStderr: true,
        Follow:     true,
        Timestamps: false,
    })
    if err != nil {
        return err
    }
    defer out.Close()

    // blocks until ctx is cancelled or container stops
    stdcopy.StdCopy(stdout, stderr, out)
    return nil
}


func (v *Vessel) StopContainer(ctx context.Context, containerID string) error {
	timeout := 10
	return v.docker.ContainerStop(ctx, containerID, container.StopOptions{Timeout: &timeout})
}
 
func (v *Vessel) RemoveContainer(ctx context.Context, containerID string) error {
	return v.docker.ContainerRemove(ctx, containerID, container.RemoveOptions{Force: true})
}
 
func (v *Vessel) InspectContainer(ctx context.Context, containerID string) (*ContainerInfo, error) {
	info, err := v.docker.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}
	return &ContainerInfo{
		ID:     info.ID,
		Status: info.State.Status,
	}, nil
}

func (v *Vessel) GetImagePort(ctx context.Context, image string) (int, error) {
    inspect, err := v.docker.ImageInspect(ctx, image)
    if err != nil {
        return 0, err
    }
    for portProto := range inspect.Config.ExposedPorts {
        // portProto is like "80/tcp" or "3000/tcp"
        parts := strings.SplitN(portProto, "/", 2)
        port, err := strconv.Atoi(parts[0])
        if err != nil {
            continue
        }
        return port, nil
    }
    return 80, nil
}

func (v *Vessel) WaitForHealthy(ctx context.Context, containerID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := v.InspectContainer(ctx, containerID)
		if err != nil {
			return err
		}
		if info.Status == "running" {
			return nil
		}
		if info.Status == "exited" || info.Status == "dead" {
			return fmt.Errorf("container %s exited unexpectedly", containerID)
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("container %s did not become healthy within %s", containerID, timeout)
}

func (v *Vessel) StreamLogs(ctx context.Context, containerID string, w io.Writer) error {
	out, err := v.docker.ContainerLogs(ctx, containerID, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     true,
		Timestamps: false,
	})
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(w, out)
	return err
}
 

func (v *Vessel) PruneImages(ctx context.Context) error {
	_, err := v.docker.ImagesPrune(ctx, filters.Args{})
	return err
}