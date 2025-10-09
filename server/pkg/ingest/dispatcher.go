package ingest

import (
	"context"
	"encoding/json"

	"progressdb/pkg/logger"
	"progressdb/pkg/models"
)

// RegisterDefaultHandlers wires a simple dispatcher for create/update/delete
// ops that inspects the payload to decide whether the operation is a
// message or a thread operation. This is a convenience for initial wiring.
func RegisterDefaultHandlers(p *Processor) {
	// dispatch create
	p.RegisterHandler(OpCreate, func(ctx context.Context, op *Op) ([]BatchEntry, error) {
		// try message
		var m models.Message
		if len(op.Payload) > 0 {
			if err := json.Unmarshal(op.Payload, &m); err == nil && m.ID != "" {
				return MessageCreateHandler(ctx, op)
			}
			// try thread
			var th models.Thread
			if err := json.Unmarshal(op.Payload, &th); err == nil && th.ID != "" {
				return ThreadCreateHandler(ctx, op)
			}
		}
		// fallback: if op.Thread is set assume message
		if op.Thread != "" {
			return MessageCreateHandler(ctx, op)
		}
		logger.Warn("dispatch_create_unknown", "op", op)
		return nil, nil
	})

	p.RegisterHandler(OpUpdate, func(ctx context.Context, op *Op) ([]BatchEntry, error) {
		// optimistic: try message update then thread update
		var m models.Message
		if len(op.Payload) > 0 {
			if err := json.Unmarshal(op.Payload, &m); err == nil && m.ID != "" {
				return MessageUpdateHandler(ctx, op)
			}
			var th models.Thread
			if err := json.Unmarshal(op.Payload, &th); err == nil && th.ID != "" {
				return ThreadUpdateHandler(ctx, op)
			}
		}
		if op.Thread != "" {
			return MessageUpdateHandler(ctx, op)
		}
		logger.Warn("dispatch_update_unknown", "op", op)
		return nil, nil
	})

	p.RegisterHandler(OpDelete, func(ctx context.Context, op *Op) ([]BatchEntry, error) {
		var m models.Message
		if len(op.Payload) > 0 {
			if err := json.Unmarshal(op.Payload, &m); err == nil && m.ID != "" {
				return MessageDeleteHandler(ctx, op)
			}
			var th models.Thread
			if err := json.Unmarshal(op.Payload, &th); err == nil && th.ID != "" {
				return ThreadDeleteHandler(ctx, op)
			}
		}
		if op.Thread != "" {
			return MessageDeleteHandler(ctx, op)
		}
		logger.Warn("dispatch_delete_unknown", "op", op)
		return nil, nil
	})
}
