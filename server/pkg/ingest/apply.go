package ingest

import (
	"time"

	"progressdb/pkg/logger"
	"progressdb/pkg/store"

	"github.com/cockroachdb/pebble"
)

// applyBatchToDB converts BatchEntry list into a pebble.WriteBatch and
// applies it atomically. This uses store.ApplyBatch to hand the batch to
// the underlying DB. For now we use a simple per-entry key layout.
func applyBatchToDB(entries []BatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	wb := new(pebble.Batch)
	// Use entry.Enq as sequence if present; otherwise fall back to time-based seq.
	var seq uint64 = uint64(time.Now().UnixNano() % 1000000)
	for _, e := range entries {
		if e.Enq != 0 {
			seq = e.Enq
		}
		// build message key: thread:<threadID>:msg:<ts>-<seq>
		key, kerr := store.MsgKey(e.Thread, e.TS, seq)
		if kerr != nil {
			logger.Error("apply_batch_invalid_key", "err", kerr)
			continue
		}
		// set message value
		wb.Set([]byte(key), e.Payload, pebble.NoSync)

		// version index: version:msg:<msgID>:<ts>-<seq>
		if e.MsgID != "" {
			ik, ikerr := store.VersionKey(e.MsgID, e.TS, seq)
			if ikerr == nil {
				wb.Set([]byte(ik), e.Payload, pebble.NoSync)
			}
			// latest pointer
			lk := []byte("latest:msg:" + e.MsgID)
			wb.Set(lk, e.Payload, pebble.NoSync)
		}
		seq++
	}

	// Apply batch via store wrapper (sync=false for group commit control)
	if err := store.ApplyBatch(wb, false); err != nil {
		logger.Error("apply_batch_failed", "err", err)
		return err
	}
	// record writes for group fsync accounting
	store.RecordWrite(len(entries))
	return nil
}
