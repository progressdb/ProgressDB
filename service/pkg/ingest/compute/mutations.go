package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	qpkg "progressdb/pkg/ingest/queue"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/models"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/timeutil"
)

// message op methods
func MutMessageCreate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if len(op.Payload) == 0 {
		return nil, fmt.Errorf("empty payload for message create")
	}
	var m models.Message
	if err := json.Unmarshal(op.Payload, &m); err != nil {
		return nil, fmt.Errorf("invalid message json: %w", err)
	}
	// reconcile id/thread from op if present
	if m.ID == "" && op.MID != "" {
		m.ID = op.MID
	}
	if m.Thread == "" && op.TID != "" {
		m.Thread = op.TID
	}
	if m.TS == 0 && op.TS != 0 {
		m.TS = op.TS
	}
	// prefer extras identity as author if payload missing
	if m.Author == "" {
		if a := op.Extras["user_id"]; a != "" {
			m.Author = a
		}
	}
	if m.TS == 0 {
		m.TS = timeutil.Now().UnixNano()
	}
	if m.ID == "" {
		return nil, fmt.Errorf("missing message id")
	}

	// validate meets intake requirements
	if err := ValidateMessage(m); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// re-marshal to ensure canonical payload
	var payload []byte
	var err error
	payload, err = json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// encrypt if enabled (handled internally by EncryptMessageData)
	tr := telemetry.Track("ingest.message_encryption")
	defer tr.Finish()

	tr.Mark("encrypt")
	payload, err = encryption.EncryptMessageData(m.Thread, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	be := types.BatchEntry{Handler: qpkg.HandlerMessageCreate, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq, Model: &m}
	return []types.BatchEntry{be}, nil
}
func MutMessageUpdate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if len(op.Payload) == 0 {
		return nil, fmt.Errorf("empty payload for message update")
	}
	var m models.Message
	if err := json.Unmarshal(op.Payload, &m); err != nil {
		return nil, fmt.Errorf("invalid message json: %w", err)
	}
	if m.ID == "" && op.MID != "" {
		m.ID = op.MID
	}
	if m.Thread == "" && op.TID != "" {
		m.Thread = op.TID
	}
	if m.TS == 0 {
		m.TS = timeutil.Now().UnixNano()
	}
	// allow partial updates; caller should provide resulting full message
	var payload []byte
	var err error
	payload, err = json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal message: %w", err)
	}

	// encrypt if enabled (handled internally by EncryptMessageData)
	tr := telemetry.Track("ingest.message_encryption")
	defer tr.Finish()
	tr.Mark("encrypt")
	payload, err = encryption.EncryptMessageData(m.Thread, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	be := types.BatchEntry{Handler: qpkg.HandlerMessageUpdate, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq, Model: &m}
	return []types.BatchEntry{be}, nil
}
func MutMessageDelete(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	var m models.Message
	if len(op.Payload) > 0 {
		_ = json.Unmarshal(op.Payload, &m)
	}
	id := m.ID
	if id == "" {
		id = op.MID
	}
	if id == "" {
		return nil, fmt.Errorf("missing message id for delete")
	}
	// encode a tomb payload (minimal) so versions apply logic works
	tomb := models.Message{ID: id, Deleted: true, TS: timeutil.Now().UnixNano(), Thread: m.Thread}
	var payload []byte
	var err error
	payload, err = json.Marshal(tomb)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tomb: %w", err)
	}

	// encrypt if enabled (handled internally by EncryptMessageData)
	tr := telemetry.Track("ingest.message_encryption")
	defer tr.Finish()
	tr.Mark("encrypt")
	payload, err = encryption.EncryptMessageData(tomb.Thread, payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	be := types.BatchEntry{Handler: qpkg.HandlerMessageDelete, Thread: tomb.Thread, MsgID: id, Payload: payload, TS: tomb.TS, Enq: op.EnqSeq, Model: &tomb}
	return []types.BatchEntry{be}, nil
}

// reactions op methods
func MutReactionAdd(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if op.MID == "" {
		return nil, fmt.Errorf("missing message id for reaction")
	}
	var p struct {
		Reaction string `json:"reaction"`
		Identity string `json:"identity"`
	}
	if len(op.Payload) > 0 {
		_ = json.Unmarshal(op.Payload, &p)
	}
	identity := p.Identity
	if identity == "" && op.Extras != nil {
		identity = op.Extras["user_id"]
	}
	if p.Reaction == "" || identity == "" {
		return nil, fmt.Errorf("invalid reaction payload")
	}

	// Defer DB read to apply phase: prepare payload with reaction details
	reactionPayload := map[string]string{
		"reaction": p.Reaction,
		"identity": identity,
		"action":   "add",
	}
	payload, _ := json.Marshal(reactionPayload)
	be := types.BatchEntry{Handler: qpkg.HandlerReactionAdd, Thread: op.TID, MsgID: op.MID, Payload: payload, TS: timeutil.Now().UnixNano(), Enq: op.EnqSeq, Model: nil}
	return []types.BatchEntry{be}, nil
}
func MutReactionDelete(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if op.MID == "" {
		return nil, fmt.Errorf("missing message id for reaction delete")
	}
	var p struct {
		Remove   string `json:"remove_reaction_for"`
		Identity string `json:"identity"`
	}
	if len(op.Payload) > 0 {
		_ = json.Unmarshal(op.Payload, &p)
	}
	identity := p.Remove
	if identity == "" {
		identity = p.Identity
	}
	if identity == "" && op.Extras != nil {
		identity = op.Extras["user_id"]
	}
	if identity == "" {
		return nil, fmt.Errorf("no identity specified for reaction delete")
	}

	// Defer DB read to apply phase: prepare payload with reaction details
	reactionPayload := map[string]string{
		"identity": identity,
		"action":   "delete",
	}
	payload, _ := json.Marshal(reactionPayload)
	be := types.BatchEntry{Handler: qpkg.HandlerReactionDelete, Thread: op.TID, MsgID: op.MID, Payload: payload, TS: timeutil.Now().UnixNano(), Enq: op.EnqSeq, Model: nil}
	return []types.BatchEntry{be}, nil
}

// thread meta op methods
func MutThreadCreate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	var th models.Thread
	if len(op.Payload) == 0 {
		return nil, fmt.Errorf("empty payload for thread create")
	}
	if err := json.Unmarshal(op.Payload, &th); err != nil {
		// payload may be partial; we still accept and fill from op
		th = models.Thread{}
	}
	// If handler generated an ID (HTTP fast-path), prefer op.TID.
	if th.ID == "" && op.TID != "" {
		th.ID = op.TID
	}
	// Ensure timestamps
	if th.CreatedTS == 0 {
		th.CreatedTS = timeutil.Now().UnixNano()
	}
	if th.UpdatedTS == 0 {
		th.UpdatedTS = th.CreatedTS
	}

	// Validate thread
	if err := ValidateThread(th); err != nil {
		return nil, fmt.Errorf("thread validation failed: %w", err)
	}

	// If encryption is enabled, provision a DEK for the thread using KMS.
	tr := telemetry.Track("ingest.thread_encryption")
	defer tr.Finish()
	tr.Mark("kms_provision")
	kmsMeta, err := encryption.ProvisionThreadKMS(th.ID)
	if err != nil {
		return nil, err
	}
	th.KMS = kmsMeta

	payload, _ := json.Marshal(th)
	be := types.BatchEntry{Handler: qpkg.HandlerThreadCreate, Thread: th.ID, MsgID: "", Payload: payload, TS: th.CreatedTS, Enq: op.EnqSeq, Model: &th}
	return []types.BatchEntry{be}, nil
}
func MutThreadUpdate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	var th models.Thread
	if err := json.Unmarshal(op.Payload, &th); err != nil {
		// best-effort: fall back to op.TID
		th = models.Thread{}
	}
	if th.ID == "" && op.TID != "" {
		th.ID = op.TID
	}
	if th.UpdatedTS == 0 {
		th.UpdatedTS = timeutil.Now().UnixNano()
	}
	// Copy payload so returned types.BatchEntry does not alias the pooled op buffer.
	var payloadCopy []byte
	if len(op.Payload) > 0 {
		payloadCopy = make([]byte, len(op.Payload))
		copy(payloadCopy, op.Payload)
	}
	be := types.BatchEntry{Handler: qpkg.HandlerThreadUpdate, Thread: th.ID, Payload: payloadCopy, TS: th.UpdatedTS, Model: &th}
	return []types.BatchEntry{be}, nil
}
func MutThreadDelete(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	var th models.Thread
	if len(op.Payload) > 0 {
		_ = json.Unmarshal(op.Payload, &th)
	}
	if th.ID == "" {
		th.ID = op.TID
	}
	if th.ID == "" {
		return nil, fmt.Errorf("missing thread id for delete")
	}
	be := types.BatchEntry{Handler: qpkg.HandlerThreadDelete, Thread: th.ID, Payload: []byte{}, TS: timeutil.Now().UnixNano(), Model: nil}
	return []types.BatchEntry{be}, nil
}

func ValidateMessage(m models.Message) error {
	tr := telemetry.Track("validation.validate_message")
	defer tr.Finish()

	var errs []string
	if m.Body == nil {
		errs = append(errs, "body is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func ValidateThread(th models.Thread) error {
	tr := telemetry.Track("validation.validate_thread")
	defer tr.Finish()

	var errs []string
	if th.ID == "" {
		errs = append(errs, "id is required")
	}
	if th.Title == "" {
		errs = append(errs, "title is required")
	}
	if th.Author == "" {
		errs = append(errs, "author is required")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
