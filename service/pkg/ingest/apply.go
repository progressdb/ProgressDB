package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/messages"
	"progressdb/pkg/store/threads"
	"progressdb/pkg/telemetry"
	"progressdb/pkg/timeutil"
)

// ApplyBatchToDB persists a list of BatchEntry items to the storedb.
// Message entries are saved via storedb.SaveMessage (handles encryption and sequencing).
// Thread entries are processed with SaveThread or SoftDeleteThread as appropriate.
// Reactions are merged per message and applied.
func ApplyBatchToDB(entries []BatchEntry) error {
	tr := telemetry.Track("ingest.apply_batch")
	defer tr.Finish()

	if len(entries) == 0 {
		return nil
	}

	tr.Mark("process_entries")

	// Group reaction entries by message ID for merging
	reactionGroups := make(map[string][]BatchEntry)
	var otherEntries []BatchEntry

	for _, e := range entries {
		if e.Handler == queue.HandlerReactionAdd || e.Handler == queue.HandlerReactionDelete {
			reactionGroups[e.MsgID] = append(reactionGroups[e.MsgID], e)
		} else {
			otherEntries = append(otherEntries, e)
		}
	}

	// Process merged reactions
	for msgID, reacts := range reactionGroups {
		if err := applyMergedReactions(msgID, reacts); err != nil {
			logger.Error("apply_batch_merged_reactions_failed", "err", err, "msg", msgID)
			continue
		}
	}

	// Process other entries
	for _, e := range otherEntries {
		switch {
		case e.MsgID != "":
			// Message entry: unmarshal and save.
			var msg models.Message
			if err := json.Unmarshal(e.Payload, &msg); err != nil {
				logger.Error("apply_batch_unmarshal_message", "err", err)
				continue
			}
			// Get thread JSON for encryption
			threadJSON, err := threads.GetThread(e.Thread)
			if err != nil {
				logger.Error("apply_batch_get_thread_failed", "err", err, "thread", e.Thread)
				continue
			}
			if err := messages.SaveMessage(context.Background(), e.Thread, e.MsgID, msg, threadJSON); err != nil {
				logger.Error("apply_batch_save_message_failed", "err", err, "thread", e.Thread, "msg", e.MsgID)
				continue
			}
		default:
			// Thread-level entry. Use SoftDeleteThread for deletes, otherwise SaveThread.
			if e.Handler == queue.HandlerThreadDelete {
				if err := threads.SoftDeleteThread(e.Thread, ""); err != nil {
					logger.Error("apply_batch_soft_delete_failed", "err", err, "thread", e.Thread)
					continue
				}
			} else {
				if err := threads.SaveThread(e.Thread, string(e.Payload)); err != nil {
					logger.Error("apply_batch_save_thread_failed", "err", err, "thread", e.Thread)
					continue
				}
			}
		}
	}
	tr.Mark("record_write")
	storedb.RecordWrite(len(entries))
	return nil
}

// applyMergedReactions merges reaction ops for a message and applies them.
func applyMergedReactions(msgID string, reacts []BatchEntry) error {
	// Read the latest message once
	stored, err := messages.GetLatestMessage(msgID)
	if err != nil {
		return fmt.Errorf("message not found: %w", err)
	}
	var m models.Message
	if err := json.Unmarshal([]byte(stored), &m); err != nil {
		return fmt.Errorf("invalid stored message: %w", err)
	}
	if m.Deleted {
		return fmt.Errorf("message deleted")
	}
	if m.Reactions == nil {
		m.Reactions = make(map[string]string)
	}

	// Merge reactions: apply in order, last op wins for same identity
	for _, r := range reacts {
		var rp map[string]string
		if err := json.Unmarshal(r.Payload, &rp); err != nil {
			logger.Error("apply_merged_reactions_unmarshal", "err", err)
			continue
		}
		identity := rp["identity"]
		action := rp["action"]
		if action == "add" {
			m.Reactions[identity] = rp["reaction"]
		} else if action == "delete" {
			delete(m.Reactions, identity)
		}
	}

	// Update timestamp and save
	m.TS = timeutil.Now().UnixNano()
	// Get thread JSON for encryption
	threadJSON, err := threads.GetThread(m.Thread)
	if err != nil {
		return fmt.Errorf("get thread failed: %w", err)
	}
	return messages.SaveMessage(context.Background(), m.Thread, m.ID, m, threadJSON)
}
