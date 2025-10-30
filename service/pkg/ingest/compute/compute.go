// Performs stateless computation of mutative payloads

package compute

import (
	"context"
	"encoding/json"
	"fmt"

	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/features/threads"
)

// thread meta op methods
func ComputeThreadCreate(ctx context.Context, op *types.QueueOp) ([]types.BatchEntry, error) {
	if op.Payload == nil {
		return nil, fmt.Errorf("empty payload for thread create")
	}

	// parse
	thread, ok := op.Payload.(*models.Thread)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for thread create")
	}

	// sync
	kmsMeta, err := encryption.ProvisionThreadKMS(thread.Key)
	if err != nil {
		logger.Error("thread_kms_provision_failed", "err", err, "thread", thread.Key, "author", thread.Author)
		return nil, fmt.Errorf("kms provision failed: %w", err)
	}
	thread.KMS = kmsMeta

	// validate
	if err := ValidateReadyForBatchEntry(thread); err != nil {
		return nil, fmt.Errorf("thread validation failed: %w", err)
	}

	// done
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeThreadUpdate(ctx context.Context, op *types.QueueOp) ([]types.BatchEntry, error) {
	// parse
	update, ok := op.Payload.(*models.ThreadUpdatePartial)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for thread update")
	}

	// resolve
	threadData, err := threads.GetThread(update.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve thread: %w", err)
	}

	var existingThread models.Thread
	if err := json.Unmarshal([]byte(threadData), &existingThread); err != nil {
		return nil, fmt.Errorf("failed to unmarshal thread: %w", err)
	}

	// check if user owns the thread
	if existingThread.Author != op.Extras.UserID {
		return nil, fmt.Errorf("user not authorized to update this thread")
	}

	// validate
	if err := ValidateReadyForBatchEntry(update); err != nil {
		return nil, fmt.Errorf("thread update validation failed: %w", err)
	}

	// done
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeThreadDelete(ctx context.Context, op *types.QueueOp) ([]types.BatchEntry, error) {
	// parse
	del, ok := op.Payload.(*models.ThreadDeletePartial)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for thread delete")
	}

	// resolve
	threadData, err := threads.GetThread(del.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve thread: %w", err)
	}

	var thread models.Thread
	if err := json.Unmarshal([]byte(threadData), &thread); err != nil {
		return nil, fmt.Errorf("failed to unmarshal thread: %w", err)
	}

	// check if user owns the thread
	if thread.Author != op.Extras.UserID {
		return nil, fmt.Errorf("user not authorized to delete this thread")
	}

	// done
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}

// message op methods
func ComputeMessageCreate(ctx context.Context, op *types.QueueOp) ([]types.BatchEntry, error) {
	if op.Payload == nil {
		return nil, fmt.Errorf("empty payload for message create")
	}

	// parse
	message, ok := op.Payload.(*models.Message)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for message create")
	}

	// validate
	if err := ValidateReadyForBatchEntry(message); err != nil {
		return nil, fmt.Errorf("message create validation failed: %w", err)
	}

	// done
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeMessageUpdate(ctx context.Context, op *types.QueueOp) ([]types.BatchEntry, error) {
	if op.Payload == nil {
		return nil, fmt.Errorf("empty payload for message update")
	}

	// parse
	update, ok := op.Payload.(*models.MessageUpdatePartial)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for message update")
	}

	// validate
	if err := ValidateReadyForBatchEntry(update); err != nil {
		return nil, fmt.Errorf("message update validation failed: %w", err)
	}

	// done
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeMessageDelete(ctx context.Context, op *types.QueueOp) ([]types.BatchEntry, error) {
	// parse
	del, ok := op.Payload.(*models.MessageDeletePartial)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for message delete")
	}

	// encode a tomb payload (minimal) so versions apply logic works
	tomb := models.Message{Key: del.Key, Deleted: true, TS: del.TS, Thread: del.Thread, Author: del.Author}
	op.Payload = &tomb

	// validate
	if err := ValidateReadyForBatchEntry(&tomb); err != nil {
		return nil, fmt.Errorf("message delete validation failed: %w", err)
	}

	// done
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
