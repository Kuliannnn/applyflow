package httpapi

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterConcurrentBudgetAndExpiry(t *testing.T) {
	l := newLimiter()
	now := time.Now()
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _ := l.take("same-client", 10, time.Minute, now)
			if ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("allowed=%d", allowed.Load())
	}
	if ok, _ := l.take("other-client", 10, time.Minute, now); !ok {
		t.Fatal("unrelated client blocked")
	}
	if ok, _ := l.take("same-client", 10, time.Minute, now.Add(time.Minute)); !ok {
		t.Fatal("expired budget not reset")
	}
}
