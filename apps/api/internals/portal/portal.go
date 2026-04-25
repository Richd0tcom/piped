package portal

import (
	"sync"
	"sync/atomic"

	"github.com/richd0tcom/piped/internals/models"
	"github.com/richd0tcom/piped/internals/store"
)


type subscriber struct {
	ch chan *models.LogLine
}

type Portal struct {
	store    *store.Store
	mu       sync.RWMutex
	subs     map[string][]*subscriber
	ingest   chan *models.LogLine
	sequences sync.Map // deploymentID -> *atomic.Int64
	done     chan struct{}
}

func New(store *store.Store) *Portal {
	p := &Portal{
		store: store,
		subs:  make(map[string][]*subscriber),
		ingest: make(chan *models.LogLine, 512),
		done: make(chan struct{}),
	}

	go p.drain()
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

func (p *Portal) UnSubscribe(deploymentID string, ch chan *models.LogLine) {
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
