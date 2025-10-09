package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"progressdb/pkg/models"
)

func ThreadCreateHandler(ctx context.Context, op *Op) ([]BatchEntry, error) {
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
		th.CreatedTS = time.Now().UTC().UnixNano()
	}
	if th.UpdatedTS == 0 {
		th.UpdatedTS = th.CreatedTS
	}
	be := BatchEntry{Type: OpCreate, Thread: th.ID, MsgID: "", Payload: op.Payload, TS: th.CreatedTS}
	return []BatchEntry{be}, nil
}

func ThreadUpdateHandler(ctx context.Context, op *Op) ([]BatchEntry, error) {
	var th models.Thread
	if err := json.Unmarshal(op.Payload, &th); err != nil {
		// best-effort: fall back to op.Thread
		th = models.Thread{}
	}
	if th.ID == "" && op.Thread != "" {
		th.ID = op.Thread
	}
	if th.UpdatedTS == 0 {
		th.UpdatedTS = time.Now().UTC().UnixNano()
	}
	be := BatchEntry{Type: OpUpdate, Thread: th.ID, Payload: op.Payload, TS: th.UpdatedTS}
	return []BatchEntry{be}, nil
}

func ThreadDeleteHandler(ctx context.Context, op *Op) ([]BatchEntry, error) {
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
	be := BatchEntry{Type: OpDelete, Thread: th.ID, Payload: []byte{}, TS: time.Now().UTC().UnixNano()}
	return []BatchEntry{be}, nil
}
