package file

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerLoop_ImmediateTick(t *testing.T) {
	var ticked atomic.Bool

	loop := NewWorkerLoop("test", 10*time.Second, func(ctx context.Context) {
		ticked.Store(true)
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Cancel quickly — well before the 10s interval
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	if !ticked.Load() {
		t.Fatal("expected tick to run immediately on startup")
	}
}

func TestWorkerLoop_PanicRecovery(t *testing.T) {
	var count atomic.Int32

	loop := NewWorkerLoop("test", 50*time.Millisecond, func(ctx context.Context) {
		n := count.Add(1)
		if n == 1 {
			panic("test panic")
		}
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Wait for restart + at least one more tick
	time.Sleep(6 * time.Second)
	cancel()
	<-done

	if count.Load() < 2 {
		t.Fatalf("expected at least 2 ticks (panic + recovery), got %d", count.Load())
	}
}

func TestWorkerLoop_NotifyWake(t *testing.T) {
	var ticks atomic.Int32
	notify := make(chan struct{}, 1)

	loop := NewWorkerLoop("test", 10*time.Second, func(ctx context.Context) {
		ticks.Add(1)
	}, notify)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		loop.Run(ctx)
		close(done)
	}()

	// Wait for the immediate startup tick
	time.Sleep(50 * time.Millisecond)

	// Send a notify signal — should trigger another tick without waiting 10s
	notify <- struct{}{}
	time.Sleep(50 * time.Millisecond)

	cancel()
	<-done

	// Expect at least 2 ticks: immediate startup + notify-triggered
	if ticks.Load() < 2 {
		t.Fatalf("expected at least 2 ticks, got %d", ticks.Load())
	}
}
