package api

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	qpkg "progressdb/pkg/ingest/queue"
)

func TestQueueTryEnqueueAndDrop(t *testing.T) {
	q := qpkg.NewIngestQueue(2)

	if err := q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "m1", nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "m2", nil, 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// next should fail with ErrQueueFull
	if err := q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "m3", nil, 0); err == nil {
		t.Fatalf("expected ErrQueueFull, got nil")
	}
	if q.Dropped() == 0 {
		t.Fatalf("expected dropped > 0")
	}
}

func TestQueueEnqueueBlockingAndOut(t *testing.T) {
	q := qpkg.NewIngestQueue(2)

	// start consumer
	recv := make(chan *qpkg.QueueItem, 4)
	go func() {
		for it := range q.Out() {
			recv <- it
		}
	}()

	// enqueue two items
	if err := q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "m1", []byte("a"), 0); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}
	if err := q.EnqueueBytes(qpkg.HandlerMessageUpdate, "t1", "m2", []byte("b"), 0); err != nil {
		t.Fatalf("enqueue failed: %v", err)
	}

	// allow consumer to receive
	select {
	case o := <-recv:
		if o.Op.ID != "m1" && o.Op.ID != "m2" {
			t.Fatalf("unexpected op id: %s", o.Op.ID)
		}
		o.JobDone()
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("timed out waiting for consumer")
	}
}

func TestCloseAndDrain(t *testing.T) {
	q := qpkg.NewIngestQueue(4)
	// enqueue some items
	_ = q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "a", []byte("x"), 0)
	_ = q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "b", []byte("y"), 0)

	q.Close()

	if q.Len() != 2 {
		t.Fatalf("expected queue not drained, got len=%d", q.Len())
	}
}

func TestRunWorkerEnsuresDone(t *testing.T) {
	q := qpkg.NewIngestQueue(4)
	stop := make(chan struct{})
	processed := make(chan string, 4)
	go q.RunWorker(stop, func(op *qpkg.QueueOp) error {
		processed <- op.ID
		return nil
	})

	_ = q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "x", []byte("p"), 0)
	_ = q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "y", []byte("q"), 0)

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

func TestQueueCloseWaitsForDrain(t *testing.T) {
	q := qpkg.NewIngestQueue(8)
	stop := make(chan struct{})
	var processed int32

	go q.RunWorker(stop, func(op *qpkg.QueueOp) error {
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&processed, 1)
		return nil
	})

	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("m%d", i)
		if err := q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", id, []byte("p"), 0); err != nil {
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

	if err := q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", "late", nil, 0); err != qpkg.ErrQueueClosed {
		t.Fatalf("expected ErrQueueClosed, got %v", err)
	}

	close(stop)
}

func TestRunBatchWorkerBatches(t *testing.T) {
	q := qpkg.NewIngestQueue(16)
	stop := make(chan struct{})
	batchCh := make(chan []string, 4)

	go q.RunBatchWorker(stop, 3, func(ops []*qpkg.QueueOp) error {
		ids := make([]string, len(ops))
		for i, op := range ops {
			ids[i] = op.ID
		}
		batchCh <- ids
		return nil
	})

	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("b%d", i)
		if err := q.EnqueueBytes(qpkg.HandlerMessageCreate, "t1", id, []byte("x"), 0); err != nil {
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

	if len(batches) == 0 {
		t.Fatalf("expected batches to be processed")
	}

	total := 0
	for _, b := range batches {
		if len(b) > 3 {
			t.Fatalf("batch size exceeded limit: %v", b)
		}
		total += len(b)
	}
	if total != 5 {
		t.Fatalf("expected 5 ops processed, got %d", total)
	}

	close(stop)
}
