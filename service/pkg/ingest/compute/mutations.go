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
func MutThreadCreate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	if len(op.Payload) == 0 {
		return nil, fmt.Errorf("empty payload for thread create")
	}

	// parse
	var th models.Thread
	if err := json.Unmarshal(op.Payload, &th); err != nil {
		th = models.Thread{}
	}

	// timestamps
	if th.CreatedTS == 0 {
		th.CreatedTS = timeutil.Now().UnixNano()
	}
	if th.UpdatedTS == 0 {
		th.UpdatedTS = th.CreatedTS
	}

	// validate
	if err := ValidateThread(th, ValidationTypeCreate); err != nil {
		logger.Error("thread_create_validation_failed", "err", err, "author", th.Author, "title", th.Title)
		return nil, fmt.Errorf("thread validation failed: %w", err)
	}

	// enc DEK
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
	payload, _ := json.Marshal(th)
	be := types.BatchEntry{Handler: qpkg.HandlerThreadCreate, TID: op.TID, Payload: payload, TS: th.CreatedTS, Enq: op.EnqSeq, Model: &th}
	return []types.BatchEntry{be}, nil
}
func MutThreadUpdate(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
	var th models.Thread
	if err := json.Unmarshal(op.Payload, &th); err != nil {
		// best-effort: fall back to op.TID
		th = models.Thread{}
	}

	// sync
	th.ID = op.TID
	th.UpdatedTS = timeutil.Now().UnixNano()

	// copy payload so returned types.BatchEntry does not alias the pooled op buffer.
	var payloadCopy []byte
	if len(op.Payload) > 0 {
		payloadCopy = make([]byte, len(op.Payload))
		copy(payloadCopy, op.Payload)
	}
	be := types.BatchEntry{Handler: qpkg.HandlerThreadUpdate, TID: th.ID, Payload: payloadCopy, TS: th.UpdatedTS, Model: &th}
	return []types.BatchEntry{be}, nil
}
func MutThreadDelete(ctx context.Context, op *qpkg.QueueOp) ([]types.BatchEntry, error) {
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
	be := types.BatchEntry{Handler: qpkg.HandlerThreadDelete, TID: op.TID, Payload: []byte{}, TS: timeutil.Now().UnixNano(), Model: nil, Author: userID}
	return []types.BatchEntry{be}, nil
}

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
	m.ID = op.MID
	m.Thread = op.TID
	m.TS = op.TS

	// prefer extras identity as author if payload missing
	if m.ID == "" || m.Author == "" {
		return nil, fmt.Errorf("missing message id")
	}

	// validate meets intake requirements
	if err := ValidateMessage(m); err != nil {
		logger.Error("message_validation_failed", "err", err, "author", m.Author, "thread", m.Thread)
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
		logger.Error("message_encryption_failed", "err", err, "thread", m.Thread, "msg", m.ID)
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	be := types.BatchEntry{Handler: qpkg.HandlerMessageCreate, TID: m.Thread, MID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq, Model: &m}
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

	// set to defaults
	m.ID = op.MID
	m.Thread = op.TID

	// if timestamp is still not set
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
		logger.Error("message_encryption_failed", "err", err, "thread", m.Thread, "msg", m.ID)
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	be := types.BatchEntry{Handler: qpkg.HandlerMessageUpdate, TID: m.Thread, MID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq, Model: &m}
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

	be := types.BatchEntry{Handler: qpkg.HandlerMessageDelete, TID: tomb.Thread, MID: id, Payload: payload, TS: tomb.TS, Enq: op.EnqSeq, Model: &tomb}
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
	if identity == "" && op.Extras.UserID != "" {
		identity = op.Extras.UserID
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
	be := types.BatchEntry{Handler: qpkg.HandlerReactionAdd, TID: op.TID, MID: op.MID, Payload: payload, TS: timeutil.Now().UnixNano(), Enq: op.EnqSeq, Model: nil}
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
	if identity == "" && op.Extras.UserID != "" {
		identity = op.Extras.UserID
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
	be := types.BatchEntry{Handler: qpkg.HandlerReactionDelete, TID: op.TID, MID: op.MID, Payload: payload, TS: timeutil.Now().UnixNano(), Enq: op.EnqSeq, Model: nil}
	return []types.BatchEntry{be}, nil
}

// others
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
