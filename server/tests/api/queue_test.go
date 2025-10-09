package api

import (
	"context"
	"testing"
	"time"
)

func TestQueueTryEnqueueAndDrop(t *testing.T) {
	q := NewQueue(2)

	if err := q.TryEnqueueBytes(OpCreate, "t1", "m1", nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := q.TryEnqueueBytes(OpCreate, "t1", "m2", nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// next should fail with ErrQueueFull
	if err := q.TryEnqueueBytes(OpCreate, "t1", "m3", nil, 0); err == nil {
		t.Fatalf("expected ErrQueueFull, got nil")
	}
	if q.Dropped() == 0 {
		t.Fatalf("expected dropped > 0")
	}
}

func TestQueueEnqueueBlockingAndOut(t *testing.T) {
	q := NewQueue(2)

	// start consumer
	recv := make(chan *Item, 4)
	go func() {
		for it := range q.Out() {
			recv <- it
		}
	}()

	// enqueue two items
	ctx := context.Background()
	if err := q.EnqueueBytes(ctx, OpCreate, "t1", "m1", []byte("a"), 0); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if err := q.EnqueueBytes(ctx, OpUpdate, "t1", "m2", []byte("b"), 0); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	// allow consumer to receive
	select {
	case o := <-recv:
		if o.Op.ID != "m1" && o.Op.ID != "m2" {
			t.Fatalf("unexpected op id: %s", o.Op.ID)
		}
		o.Done()
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for consumer")
	}
}

func TestEnqueueWithContextCancel(t *testing.T) {
	q := NewQueue(1)
	// fill queue
	if err := q.TryEnqueueBytes(OpCreate, "t1", "m1", nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := q.EnqueueBytes(ctx, OpCreate, "t1", "m2", nil, 0)
	if err == nil {
		t.Fatalf("expected enqueue to fail due to cancelled context")
	}
}

func TestCloseAndDrain(t *testing.T) {
	q := NewQueue(4)
	// enqueue some items
	_ = q.TryEnqueueBytes(OpCreate, "t1", "a", []byte("x"), 0)
	_ = q.TryEnqueueBytes(OpCreate, "t1", "b", []byte("y"), 0)

	q.CloseAndDrain()

	if q.Len() != 0 {
		t.Fatalf("expected queue drained, got len=%d", q.Len())
	}
}

func TestRunWorkerEnsuresDone(t *testing.T) {
	q := NewQueue(4)
	stop := make(chan struct{})
	processed := make(chan string, 4)
	go q.RunWorker(stop, func(op *Op) error {
		processed <- op.ID
		return nil
	})

	_ = q.TryEnqueueBytes(OpCreate, "t1", "x", []byte("p"), 0)
	_ = q.TryEnqueueBytes(OpCreate, "t1", "y", []byte("q"), 0)

	// allow worker to process
	select {
	case id := <-processed:
		if id == "" {
			t.Fatalf("unexpected empty id")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("worker did not process item")
	}

	close(stop)
}
