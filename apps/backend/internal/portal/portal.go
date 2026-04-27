package portal

import (
	"sync"
	"sync/atomic"

	"github.com/richd0tcom/piped/internal/models"
	"github.com/richd0tcom/piped/internal/store"
)


type subscriber struct {
	ch chan *models.LogLine
}

type statusSubscriber struct {
	ch chan string
}

type Portal struct {
	store         *store.Store
	mu            sync.RWMutex
	subs          map[string][]*subscriber
	statusSubs    map[string][]*statusSubscriber
	ingest        chan *models.LogLine
	statusIngest  chan string
	sequences     sync.Map // deploymentID -> *atomic.Int64
	done          chan struct{}
}

func New(store *store.Store) *Portal {
	p := &Portal{
		store:        store,
		subs:         make(map[string][]*subscriber),
		statusSubs:   make(map[string][]*statusSubscriber),
		ingest:       make(chan *models.LogLine, 512),
		statusIngest: make(chan string, 128),
		done:         make(chan struct{}),
	}

	go p.drain()
	go p.drainStatus()
	return p
}

func (p *Portal) Subscribe(deploymentID string) chan *models.LogLine {
	sub := &subscriber{ch: make(chan *models.LogLine, 128)}
	p.mu.Lock()

	//can the same channel be appended multiple times???
	p.subs[deploymentID] = append(p.subs[deploymentID], sub)
	p.mu.Unlock()
	return sub.ch
}

func (p *Portal) Unsubscribe(deploymentID string, ch chan *models.LogLine) {
	p.mu.Lock()
	defer p.mu.Unlock()

	subs := p.subs[deploymentID]

	for i, s := range subs{
		if s.ch == ch {
			p.subs[deploymentID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (p *Portal) SubscribeStatus(deploymentID string) chan string {
	sub := &statusSubscriber{ch: make(chan string, 32)}
	p.mu.Lock()
	p.statusSubs[deploymentID] = append(p.statusSubs[deploymentID], sub)
	p.mu.Unlock()
	return sub.ch
}

func (p *Portal) UnsubscribeStatus(deploymentID string, ch chan string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	subs := p.statusSubs[deploymentID]

	for i, s := range subs{
		if s.ch == ch {
			p.statusSubs[deploymentID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			return
		}
	}
}

func (p *Portal) Close() {
	close(p.done)
}

func (p *Portal) nextSeq(deploymentID string) int64 {
	v, _ := p.sequences.LoadOrStore(deploymentID, &atomic.Int64{})
	return v.(*atomic.Int64).Add(1)
}

func (p *Portal) fanOut(line *models.LogLine) {
	p.mu.RLock()
	subs := p.subs[line.DeploymentID]
	p.mu.RUnlock()
	for _, s := range subs {
		select {
		case s.ch <- line:
		default:
			
		}
	}
}


func (p *Portal) drain() {
	for {
		select {
		case line := <-p.ingest:
			p.store.InsertLogLine(line)
			p.fanOut(line)
		case <-p.done:
			return
		}
	}
}

func (p *Portal) drainStatus() {
	for {
		select {
		case status := <-p.statusIngest:
			p.fanOutStatus(status)
		case <-p.done:
			return
		}
	}
}

func (p *Portal) fanOutStatus(status string) {
	// Parse deploymentID from status format: "deploymentID:status"
	// Find the colon separator
	for i, r := range status {
		if r == ':' {
			deploymentID := status[:i]
			statusValue := status[i+1:]
			
			p.mu.RLock()
			subs := p.statusSubs[deploymentID]
			p.mu.RUnlock()
			
			for _, s := range subs {
				select {
				case s.ch <- statusValue:
				default:
					// Channel full, drop status update
				}
			}
			return
		}
	}
}

func (p *Portal) Publish(entry models.LogEntry) {
	sq:= p.nextSeq(entry.DeploymentID)

	line:= &models.LogLine{
		DeploymentID: entry.DeploymentID,
		Stream: entry.Stream,
		Text: entry.Text,
		Sequence: sq,
	}

	p.ingest <- line
}

func (p *Portal) PublishStatus(deploymentID, status string) {
	p.statusIngest <- deploymentID + ":" + status
}
