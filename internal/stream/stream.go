// Package stream implements an in-process broadcaster for live post
// updates delivered over Server-Sent Events.
package stream

import (
	"encoding/json"
	"sync"
)

// Hub fans out new posts to subscribers. It is safe for concurrent use.
type Hub struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[chan []byte]struct{})}
}

// Subscribe registers a subscriber and returns the channel on which
// events will be delivered. The channel is buffered; slow subscribers
// are dropped rather than blocking the server.
func (h *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 64)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber.
func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	delete(h.subs, ch)
	h.mu.Unlock()
}

// Publish serializes an event and delivers it to all subscribers.
func (h *Hub) Publish(event string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg := []byte("event: " + event + "\ndata: " + string(raw) + "\n\n")
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs {
		select {
		case ch <- msg:
		default:
			// Slow subscriber: drop it.
			delete(h.subs, ch)
			close(ch)
		}
	}
}
