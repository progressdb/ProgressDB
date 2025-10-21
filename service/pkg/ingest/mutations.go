package ingest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"

	qpkg "progressdb/pkg/ingest/queue"
	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/store/messages"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/timeutil"
)

// message op methods
func MutMessageCreate(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
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
	if err := ValidateMessage(m); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}
	// re-marshal to ensure canonical payload
	payload, _ := json.Marshal(m)
	be := BatchEntry{Handler: qpkg.HandlerMessageCreate, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}
func MutMessageUpdate(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
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
		m.TS = timeutil.Now().UnixNano()
	}
	// allow partial updates; caller should provide resulting full message
	payload, _ := json.Marshal(m)
	be := BatchEntry{Handler: qpkg.HandlerMessageUpdate, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}
func MutMessageDelete(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
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
	tomb := models.Message{ID: id, Deleted: true, TS: timeutil.Now().UnixNano(), Thread: m.Thread}
	payload, _ := json.Marshal(tomb)
	be := BatchEntry{Handler: qpkg.HandlerMessageDelete, Thread: tomb.Thread, MsgID: id, Payload: payload, TS: tomb.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}

// reactions op methods
func MutReactionAdd(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
	if op.ID == "" {
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

	// load latest message
	stored, err := messages.GetLatestMessage(op.ID)
	if err != nil {
		return nil, fmt.Errorf("message not found: %w", err)
	}
	var m models.Message
	if err := json.Unmarshal([]byte(stored), &m); err != nil {
		return nil, fmt.Errorf("invalid stored message: %w", err)
	}
	if m.Deleted {
		return nil, fmt.Errorf("message deleted")
	}
	if m.Reactions == nil {
		m.Reactions = make(map[string]string)
	}
	m.Reactions[identity] = p.Reaction
	m.TS = timeutil.Now().UnixNano()
	payload, _ := json.Marshal(m)
	be := BatchEntry{Handler: qpkg.HandlerReactionAdd, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}
func MutReactionDelete(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
	if op.ID == "" {
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

	stored, err := messages.GetLatestMessage(op.ID)
	if err != nil {
		return nil, fmt.Errorf("message not found: %w", err)
	}
	var m models.Message
	if err := json.Unmarshal([]byte(stored), &m); err != nil {
		return nil, fmt.Errorf("invalid stored message: %w", err)
	}
	if m.Reactions != nil {
		delete(m.Reactions, identity)
	}
	m.TS = timeutil.Now().UnixNano()
	payload, _ := json.Marshal(m)
	be := BatchEntry{Handler: qpkg.HandlerReactionDelete, Thread: m.Thread, MsgID: m.ID, Payload: payload, TS: m.TS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}

// thread meta op methods
func MutThreadCreate(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
	var th models.Thread
	if len(op.Payload) == 0 {
		return nil, fmt.Errorf("empty payload for thread create")
	}
	if err := json.Unmarshal(op.Payload, &th); err != nil {
		// payload may be partial; we still accept and fill from op
		th = models.Thread{}
	}
	// If handler generated an ID (HTTP fast-path), prefer op.Thread.
	if th.ID == "" && op.Thread != "" {
		th.ID = op.Thread
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
	if security.EncryptionEnabled() {
		if kms.IsProviderEnabled() {
			logger.Info("ingest_provisioning_thread_kms", "thread", th.ID)
			tr.Mark("kms_create_dek")
			keyID, wrapped, kekID, kekVer, err := kms.CreateDEKForThread(th.ID)
			if err != nil {
				return nil, fmt.Errorf("kms provision failed: %w", err)
			}
			th.KMS = &models.KMSMeta{KeyID: keyID, WrappedDEK: base64.StdEncoding.EncodeToString(wrapped), KEKID: kekID, KEKVersion: kekVer}
		} else {
			logger.Info("encryption_enabled_but_no_kms_provider", "thread", th.ID)
		}
	}

	payload, _ := json.Marshal(th)
	be := BatchEntry{Handler: qpkg.HandlerThreadCreate, Thread: th.ID, MsgID: "", Payload: payload, TS: th.CreatedTS, Enq: op.EnqSeq}
	return []BatchEntry{be}, nil
}
func MutThreadUpdate(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
	var th models.Thread
	if err := json.Unmarshal(op.Payload, &th); err != nil {
		// best-effort: fall back to op.Thread
		th = models.Thread{}
	}
	if th.ID == "" && op.Thread != "" {
		th.ID = op.Thread
	}
	if th.UpdatedTS == 0 {
		th.UpdatedTS = timeutil.Now().UnixNano()
	}
	// Copy payload so returned BatchEntry does not alias the pooled Op buffer.
	var payloadCopy []byte
	if len(op.Payload) > 0 {
		payloadCopy = make([]byte, len(op.Payload))
		copy(payloadCopy, op.Payload)
	}
	be := BatchEntry{Handler: qpkg.HandlerThreadUpdate, Thread: th.ID, Payload: payloadCopy, TS: th.UpdatedTS}
	return []BatchEntry{be}, nil
}
func MutThreadDelete(ctx context.Context, op *qpkg.QueueOp) ([]BatchEntry, error) {
	var th models.Thread
	if len(op.Payload) > 0 {
		_ = json.Unmarshal(op.Payload, &th)
	}
	if th.ID == "" {
		th.ID = op.Thread
	}
	if th.ID == "" {
		return nil, fmt.Errorf("missing thread id for delete")
	}
	be := BatchEntry{Handler: qpkg.HandlerThreadDelete, Thread: th.ID, Payload: []byte{}, TS: timeutil.Now().UnixNano()}
	return []BatchEntry{be}, nil
}
