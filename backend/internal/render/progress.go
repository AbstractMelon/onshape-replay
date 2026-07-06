package render

import (
	"sync"
)

// ProgressBroadcaster fans a single progress source out to multiple SSE
// subscribers. Each subscriber gets its own buffered channel.
type ProgressBroadcaster struct {
	mu          sync.Mutex
	subscribers []chan ProgressSnapshot
	closed      bool
}

// newProgressBroadcaster creates a new broadcaster.
func newProgressBroadcaster() *ProgressBroadcaster {
	return &ProgressBroadcaster{}
}

// Subscribe registers a new subscriber and returns a receive-only channel.
// The channel is closed when the broadcaster is closed (job reaches terminal
// state). Subscribers should drain and discard after close.
func (b *ProgressBroadcaster) Subscribe() <-chan ProgressSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan ProgressSnapshot, 16)
	if b.closed {
		close(ch)
		return ch
	}
	b.subscribers = append(b.subscribers, ch)
	return ch
}

// Publish sends a snapshot to all current subscribers.
// Slow subscribers are skipped (non-blocking send with drop).
func (b *ProgressBroadcaster) Publish(snap ProgressSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- snap:
		default:
			// Drop the update for a slow subscriber rather than blocking the
			// pipeline goroutine.
		}
	}
}

// Close signals all subscribers that the job is done and removes them.
func (b *ProgressBroadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for _, ch := range b.subscribers {
		close(ch)
	}
	b.subscribers = nil
}
