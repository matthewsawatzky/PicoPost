package stream

import (
	"strings"
	"testing"
	"time"
)

func TestPublishDelivers(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe()
	defer h.Unsubscribe(ch)

	h.Publish("post", map[string]any{"id": "p_1", "channel": "chat/general"})

	select {
	case msg := <-ch:
		s := string(msg)
		if !strings.Contains(s, "event: post") {
			t.Errorf("missing event line: %q", s)
		}
		if !strings.Contains(s, `"id":"p_1"`) {
			t.Errorf("missing payload: %q", s)
		}
	case <-time.After(time.Second):
		t.Fatal("no message delivered")
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe()
	h.Unsubscribe(ch)
	h.Publish("post", map[string]any{"id": "p_1"})
	select {
	case msg := <-ch:
		t.Fatalf("unexpected message after unsubscribe: %q", msg)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSlowSubscriberDropped(t *testing.T) {
	h := NewHub()
	ch := h.Subscribe()
	// Fill the buffer without reading.
	for i := 0; i < 100; i++ {
		h.Publish("post", map[string]any{"id": "p"})
	}
	// The slow subscriber should have been dropped; publishing must not block.
	done := make(chan struct{})
	go func() {
		h.Publish("post", map[string]any{"id": "p"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publish blocked on slow subscriber")
	}
	_ = ch
}
