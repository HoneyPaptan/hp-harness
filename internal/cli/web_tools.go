package cli

import (
	"sync"
	"time"
)

const webIdleTimeout = 10 * time.Minute

type parkedTools struct {
	tools *toolset
	timer *time.Timer
}

type toolPool struct {
	mu     sync.Mutex
	idle   time.Duration
	parked map[string]*parkedTools
	closed bool
}

func newToolPool(idle time.Duration) *toolPool {
	return &toolPool{idle: idle, parked: make(map[string]*parkedTools)}
}

func (p *toolPool) take(id string) *toolset {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.parked[id]
	if entry == nil {
		return nil
	}
	delete(p.parked, id)
	entry.timer.Stop()
	return entry.tools
}

func (p *toolPool) park(id string, tools *toolset) {
	if p.store(id, tools) {
		return
	}
	tools.close()
}

func (p *toolPool) store(id string, tools *toolset) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.parked[id] != nil {
		return false
	}
	entry := &parkedTools{tools: tools}
	entry.timer = time.AfterFunc(p.idle, func() { p.expire(id, entry) })
	p.parked[id] = entry
	return true
}

func (p *toolPool) expire(id string, entry *parkedTools) {
	if p.remove(id, entry) {
		entry.tools.close()
	}
}

func (p *toolPool) remove(id string, entry *parkedTools) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.parked[id] != entry {
		return false
	}
	delete(p.parked, id)
	return true
}

func (p *toolPool) close() {
	for _, entry := range p.drain() {
		entry.timer.Stop()
		entry.tools.close()
	}
}

func (p *toolPool) drain() map[string]*parkedTools {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	out := p.parked
	p.parked = make(map[string]*parkedTools)
	return out
}
