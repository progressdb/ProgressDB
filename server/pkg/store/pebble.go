package store

import (
    "bytes"
    "fmt"
    "sync/atomic"
    "time"

    "github.com/cockroachdb/pebble"
)

var db *pebble.DB

// seq provides a small counter to reduce key collisions when multiple
// messages share the same nanosecond timestamp.
var seq uint64

// Open opens (or creates) a Pebble database at the given path and keeps
// a global handle for simple usage in this package.
func Open(path string) error {
    var err error
    db, err = pebble.Open(path, &pebble.Options{})
    return err
}

// SaveMessage appends a message to a thread by inserting a new key with
// a sortable timestamp prefix. Messages are ordered by insertion time.
func SaveMessage(threadID, msg string) error {
    if db == nil {
        return fmt.Errorf("pebble not opened; call store.Open first")
    }
    // Key format: thread:<threadID>:<unix_nano_padded>-<seq>
    ts := time.Now().UTC().UnixNano()
    s := atomic.AddUint64(&seq, 1)
    key := fmt.Sprintf("thread:%s:%020d-%06d", threadID, ts, s)
    return db.Set([]byte(key), []byte(msg), pebble.Sync)
}

// ListMessages returns all messages for a thread in insertion order.
func ListMessages(threadID string) ([]string, error) {
    if db == nil {
        return nil, fmt.Errorf("pebble not opened; call store.Open first")
    }
    prefix := []byte("thread:" + threadID + ":")
    iter, err := db.NewIter(&pebble.IterOptions{})
    if err != nil {
        return nil, err
    }
    defer iter.Close()

    var out []string
    for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
        if !bytes.HasPrefix(iter.Key(), prefix) {
            break
        }
        // Capture the value before advancing.
        v := append([]byte(nil), iter.Value()...)
        out = append(out, string(v))
    }
    return out, iter.Error()
}

