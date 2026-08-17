// Package worker runs the in-process event workers: a buffered channel bus
// plus a small goroutine pool. Swap for SQS/Redis streams by re-implementing
// events.Bus — publishers never change.
package worker

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/chaosapp/backend/internal/events"
)

type Pool struct {
	queue    chan events.Event
	handlers map[string][]events.Handler
	log      *zap.Logger
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

var _ events.Bus = (*Pool)(nil)

func New(log *zap.Logger, buffer int) *Pool {
	return &Pool{
		queue:    make(chan events.Event, buffer),
		handlers: make(map[string][]events.Handler),
		log:      log,
	}
}

// Subscribe registers a handler for an event name (call before Start).
func (p *Pool) Subscribe(name string, h events.Handler) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[name] = append(p.handlers[name], h)
}

// Publish enqueues without blocking the request path; drops (with a log) if
// the buffer is full — acceptable for non-critical side effects.
func (p *Pool) Publish(e events.Event) {
	select {
	case p.queue <- e:
	default:
		p.log.Warn("event queue full — dropping event", zap.String("event", e.Name))
	}
}

// Start launches n workers; they drain until Stop.
func (p *Pool) Start(n int) {
	for i := 0; i < n; i++ {
		p.wg.Add(1)
		go p.run()
	}
}

func (p *Pool) run() {
	defer p.wg.Done()
	for e := range p.queue {
		p.mu.RLock()
		handlers := p.handlers[e.Name]
		p.mu.RUnlock()
		for _, h := range handlers {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := h(ctx, e); err != nil {
				p.log.Error("event handler failed",
					zap.String("event", e.Name), zap.Error(err))
			}
			cancel()
		}
	}
}

// Stop closes the queue and waits for in-flight work (graceful shutdown).
func (p *Pool) Stop() {
	close(p.queue)
	p.wg.Wait()
}
