package ratelimit

import (
	"testing"
	"time"
)

func TestLimiter(t *testing.T) {
	l := New(2, time.Minute)

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("first request should be allowed")
	}
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("second request should be allowed")
	}
	if ok, retry := l.Allow("a"); ok {
		t.Fatalf("third request should be denied, got ok with retry=%d", retry)
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("different key should be allowed")
	}
}

func TestLimiterDisabled(t *testing.T) {
	l := New(0, time.Minute)
	for i := 0; i < 100; i++ {
		if ok, _ := l.Allow("x"); !ok {
			t.Fatal("disabled limiter denied a request")
		}
	}
}

func TestLimiterWindowReset(t *testing.T) {
	l := New(1, 50*time.Millisecond)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("first request should be allowed")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("second request should be denied")
	}
	time.Sleep(60 * time.Millisecond)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("request after window reset should be allowed")
	}
}
