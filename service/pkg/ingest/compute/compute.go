// Performs stateless computation of mutative payloads

package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	qpkg "progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/threads"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/timeutil"
)

// thread meta op methods
func ComputeThreadCreate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if op.Payload == nil {
		return nil, fmt.Errorf("empty payload for thread create")
	}

	// parse
	th, ok := op.Payload.(*models.Thread)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for thread create")
	}

	// validate
	if err := ValidateThread(*th, ValidationTypeCreate); err != nil {
		logger.Error("thread_create_validation_failed", "err", err, "author", th.Author, "title", th.Title)
		return nil, fmt.Errorf("thread validation failed: %w", err)
	}

	// gen DEK - if needed
	tr := telemetry.Track("ingest.thread_encryption")
	defer tr.Finish()
	tr.Mark("kms_provision")
	kmsMeta, err := encryption.ProvisionThreadKMS(th.ID)
	if err != nil {
		logger.Error("thread_kms_provision_failed", "err", err, "thread", th.ID, "author", th.Author)
		return nil, err
	}
	th.KMS = kmsMeta

	// send back
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeThreadUpdate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	// verify ownership for thread update
	threadData, err := threads.GetThread(op.TID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve thread: %w", err)
	}

	var existingThread models.Thread
	if err := json.Unmarshal([]byte(threadData), &existingThread); err != nil {
		return nil, fmt.Errorf("failed to unmarshal thread: %w", err)
	}

	// check if user owns the thread
	userID := op.Extras.UserID
	if userID == "" {
		return nil, fmt.Errorf("missing user identity for authorization")
	}
	if existingThread.Author != userID {
		return nil, fmt.Errorf("user not authorized to update this thread")
	}

	// Parse partial update payload
	updatePayload, ok := op.Payload.(*models.ThreadUpdatePartial)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for thread update")
	}

	// Set updated TS if not provided
	if updatePayload.UpdatedTS == nil {
		ts := timeutil.Now().UnixNano()
		updatePayload.UpdatedTS = &ts
	}

	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeThreadDelete(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	// verify ownership
	threadData, err := threads.GetThread(op.TID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve thread: %w", err)
	}

	var thread models.Thread
	if err := json.Unmarshal([]byte(threadData), &thread); err != nil {
		return nil, fmt.Errorf("failed to unmarshal thread: %w", err)
	}

	// check if user owns the thread
	userID := op.Extras.UserID
	if userID == "" {
		return nil, fmt.Errorf("missing user identity for authorization")
	}
	if thread.Author != userID {
		return nil, fmt.Errorf("user not authorized to delete this thread")
	}

	// send back
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}

// message op methods
func ComputeMessageCreate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if op.Payload == nil {
		return nil, fmt.Errorf("empty payload for message create")
	}

	// parse
	userID := op.Extras.UserID
	if userID == "" {
		return nil, fmt.Errorf("missing user identity for authorization")
	}

	m, ok := op.Payload.(*models.Message)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for message create")
	}

	// validate
	if m.Body == nil {
		return nil, fmt.Errorf("body is required")
	}

	// sync
	m.ID = op.MID
	m.Thread = op.TID
	m.TS = op.TS
	m.Author = userID
	op.Payload = &m

	// done
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeMessageUpdate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if op.Payload == nil {
		return nil, fmt.Errorf("empty payload for message update")
	}
	if _, ok := op.Payload.(*models.MessageUpdatePartial); !ok {
		return nil, fmt.Errorf("invalid payload type for message update")
	}

	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}
func ComputeMessageDelete(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	del, ok := op.Payload.(*models.DeletePartial)
	if !ok {
		return nil, fmt.Errorf("invalid payload type for message delete")
	}
	id := del.ID
	if id == "" {
		id = op.MID
	}
	if id == "" {
		return nil, fmt.Errorf("missing message id for delete")
	}
	// encode a tomb payload (minimal) so versions apply logic works
	tomb := models.Message{ID: id, Deleted: true, TS: del.TS, Thread: del.Thread, Author: del.Author}
	op.Payload = &tomb
	be := types.BatchEntry{QueueOp: op, Enq: op.EnqSeq}
	return []types.BatchEntry{be}, nil
}

// others
type ValidationType string

const (
	ValidationTypeCreate ValidationType = "create"
	ValidationTypeUpdate ValidationType = "update"
	ValidationTypeDelete ValidationType = "delete"
)

func ValidateThread(th models.Thread, validationType ValidationType) error {
	tr := telemetry.Track("validation.validate_thread")
	defer tr.Finish()

	var errs []string

	// ID is required for update/delete, but not for create
	if validationType != ValidationTypeCreate && th.ID == "" {
		errs = append(errs, "id is required")
	}

	// Title is required for create/update, but not for delete
	if validationType != ValidationTypeDelete && th.Title == "" {
		errs = append(errs, "title is required")
	}

	// Author is required for create/update, but not for delete
	if validationType != ValidationTypeDelete && th.Author == "" {
		errs = append(errs, "author is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
