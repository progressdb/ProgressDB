package api

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
)

func TestQueueTryEnqueueAndDrop(t *testing.T) {
	q := queue.NewIngestQueue(2)

	if err := q.EnqueueBytes(types.HandlerMessageCreate, "t1", "m1", nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := q.EnqueueBytes(types.HandlerMessageCreate, "t1", "m2", nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// next should fail with ErrQueueFull
	if err := q.EnqueueBytes(types.HandlerMessageCreate, "t1", "m3", nil, 0); err == nil {
		t.Fatalf("expected ErrQueueFull, got nil")
	}
	if q.Dropped() == 0 {
		t.Fatalf("expected dropped > 0")
	}
}

func TestQueueEnqueueBlockingAndOut(t *testing.T) {
	q := queue.NewIngestQueue(2)

	// start consumer
	recv := make(chan *types.QueueItem, 4)
	go func() {
		for it := range q.Out() {
			recv <- it
		}
	}()

	// enqueue two items
	if err := q.EnqueueBytes(types.HandlerMessageCreate, "t1", "m1", []byte("a"), 0); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if err := q.EnqueueBytes(types.HandlerMessageUpdate, "t1", "m2", []byte("b"), 0); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	// allow consumer to receive
	select {
	case o := <-recv:
		if msg, ok := o.Op.Payload.(*models.Message); ok {
			if msg.Key != "m1" && msg.Key != "m2" {
				t.Fatalf("unexpected op id: %s", msg.Key)
			}
		} else {
			t.Fatalf("unexpected payload type")
		}
		o.JobDone()
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for consumer")
	}
}

func TestCloseAndDrain(t *testing.T) {
	q := queue.NewIngestQueue(4)
	// enqueue some items
	_ = q.EnqueueBytes(types.HandlerMessageCreate, "t1", "a", []byte("x"), 0)
	_ = q.EnqueueBytes(types.HandlerMessageCreate, "t1", "b", []byte("y"), 0)

	q.Close()

	if q.Len() != 2 {
		t.Fatalf("expected queue not drained, got len=%d", q.Len())
	}
}

func TestQueueOutEnsuresDone(t *testing.T) {
	q := queue.NewIngestQueue(4)
	processed := make(chan string, 4)

	go func() {
		for it := range q.Out() {
			if msg, ok := it.Op.Payload.(*models.Message); ok {
				processed <- msg.Key
			}
			it.JobDone()
		}
	}()

	_ = q.EnqueueBytes(types.HandlerMessageCreate, "t1", "x", []byte("p"), 0)
	_ = q.EnqueueBytes(types.HandlerMessageCreate, "t1", "y", []byte("q"), 0)

	// allow consumer to process
	select {
	case id := <-processed:
		if id == "" {
			t.Fatalf("unexpected empty id")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("consumer did not process item")
	}

	q.Close()
}

func TestQueueCloseWaitsForDrain(t *testing.T) {
	q := queue.NewIngestQueue(8)
	var processed int32

	go func() {
		for it := range q.Out() {
			time.Sleep(5 * time.Millisecond)
			atomic.AddInt32(&processed, 1)
			it.JobDone()
		}
	}()

	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("m%d", i)
		if err := q.EnqueueBytes(types.HandlerMessageCreate, "t1", id, []byte("p"), 0); err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}
	}

	done := make(chan struct{})
	go func() {
		q.Close()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("queue close timed out")
	}

	// wait for worker to process
	deadline := time.Now().Add(500 * time.Millisecond)
	for atomic.LoadInt32(&processed) != 3 {
		if time.Now().After(deadline) {
			t.Fatalf("worker did not process all messages, got %d", atomic.LoadInt32(&processed))
		}
		time.Sleep(1 * time.Millisecond)
	}

	if err := q.EnqueueBytes(types.HandlerMessageCreate, "t1", "late", nil, 0); err != queue.ErrQueueClosed {
		t.Fatalf("expected ErrQueueClosed, got %v", err)
	}
}

func TestQueueOutBatches(t *testing.T) {
	q := queue.NewIngestQueue(16)
	batchCh := make(chan []string, 4)

	go func() {
		var batch []string
		for it := range q.Out() {
			if msg, ok := it.Op.Payload.(*models.Message); ok {
				batch = append(batch, msg.Key)
			}
			if len(batch) == 3 {
				batchCh <- batch
				batch = nil
			}
			it.JobDone()
		}
		if len(batch) > 0 {
			batchCh <- batch
		}
	}()

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("b%d", i)
		if err := q.EnqueueBytes(types.HandlerMessageCreate, "t1", id, []byte("x"), 0); err != nil {
			t.Fatalf("enqueue failed: %v", err)
		}
	}

	q.Close()

	var batches [][]string
collect:
	for {
		select {
		case ids := <-batchCh:
			batches = append(batches, ids)
		case <-time.After(200 * time.Millisecond):
			break collect
		}
	}

	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}

	if len(batches[0]) != 3 || len(batches[1]) != 2 {
		t.Fatalf("unexpected batch sizes: %v", batches)
	}
}
