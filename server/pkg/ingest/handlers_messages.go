package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/models"
	"progressdb/pkg/validation"
)

// MessageCreateHandler prepares a BatchEntry for a message create op.
func MessageCreateHandler(ctx context.Context, op *Op) ([]BatchEntry, error) {
	if len(op.Payload) == 0 {
		return nil, fmt.Errorf("empty payload for message create")
	}
	var m models.Message
	if err := json.Unmarshal(op.Payload, &m); err != nil {
		return nil, fmt.Errorf("invalid message json: %w", err)
	}
	// reconcile id/thread from op if present
	if m.ID == "" && op.ID != "" {
		m.ID = op.ID
	}
	if m.Thread == "" && op.Thread != "" {
		m.Thread = op.Thread
	}
	if m.TS == 0 && op.TS != 0 {
		m.TS = op.TS
	}
	// prefer extras identity as author if payload missing
	if m.Author == "" {
		if a := op.Extras["identity"]; a != "" {
			m.Author = a
		}
	}
	if m.TS == 0 {
		m.TS = time.Now().UTC().UnixNano()
	}
	if m.ID == "" {
		return nil, fmt.Errorf("missing message id")
	}
	if err := validation.ValidateMessage(m); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	// re-marshal to ensure canonical payload
	payload, _ := json.Marshal(m)
	be := BatchEntry{Type: OpCreate, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}

// MessageUpdateHandler handles message updates (reactions, edits).
func MessageUpdateHandler(ctx context.Context, op *Op) ([]BatchEntry, error) {
	if len(op.Payload) == 0 {
		return nil, fmt.Errorf("empty payload for message update")
	}
	var m models.Message
	if err := json.Unmarshal(op.Payload, &m); err != nil {
		return nil, fmt.Errorf("invalid message json: %w", err)
	}
	if m.ID == "" && op.ID != "" {
		m.ID = op.ID
	}
	if m.Thread == "" && op.Thread != "" {
		m.Thread = op.Thread
	}
	if m.TS == 0 {
		m.TS = time.Now().UTC().UnixNano()
	}
	// allow partial updates; caller should provide resulting full message
	payload, _ := json.Marshal(m)
	be := BatchEntry{Type: OpUpdate, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}

// MessageDeleteHandler prepares a delete entry for a message id.
func MessageDeleteHandler(ctx context.Context, op *Op) ([]BatchEntry, error) {
	var m models.Message
	if len(op.Payload) > 0 {
		_ = json.Unmarshal(op.Payload, &m)
	}
	id := m.ID
	if id == "" {
		id = op.ID
	}
	if id == "" {
		return nil, fmt.Errorf("missing message id for delete")
	}
	// encode a tomb payload (minimal) so versions apply logic works
	tomb := models.Message{ID: id, Deleted: true, TS: time.Now().UTC().UnixNano(), Thread: m.Thread}
	payload, _ := json.Marshal(tomb)
	be := BatchEntry{Type: OpDelete, Thread: tomb.Thread, MsgID: id, Payload: payload, TS: tomb.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}
