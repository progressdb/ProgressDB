package locks

import (
	"bytes"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

// scans messages for thread and returns largest sequence number
func ComputeMaxSeq(threadID string) (uint64, error) {
	mp, merr := keys.MsgPrefix(threadID)
	if merr != nil {
		return 0, merr
	}
	prefix := []byte(mp)
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return 0, err
	}
	defer iter.Close()
	var max uint64
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := string(iter.Key())
		_, _, s, perr := keys.ParseMsgKey(k)
		if perr != nil {
			continue
		}
		if s > max {
			max = s
		}
	}
	return max, iter.Error()
}

// wrapper for ComputeMaxSeq (for migrations/admin)
func MaxSeqForThread(threadID string) (uint64, error) {
	return ComputeMaxSeq(threadID)
}
