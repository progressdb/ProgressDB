package store

import (
    "bytes"
    "fmt"
    "sync/atomic"
    "time"

    "github.com/cockroachdb/pebble"
    "progressdb/pkg/security"
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
    data := []byte(msg)
    if security.Enabled() {
        if security.HasFieldPolicy() {
            if out, err := security.EncryptJSONFields(data); err == nil {
                data = out
            } else {
                // Fallback: full-message encryption if not JSON
                enc, err := security.Encrypt(data)
                if err != nil {
                    return err
                }
                data = enc
            }
        } else {
            enc, err := security.Encrypt(data)
            if err != nil {
                return err
            }
            data = enc
        }
    }
    return db.Set([]byte(key), data, pebble.Sync)
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
        if security.Enabled() {
            if security.HasFieldPolicy() {
                // Try full-message decrypt first; if it fails, attempt field-level decrypt.
                if dec, err := security.Decrypt(v); err == nil {
                    v = dec
                } else {
                    if outJSON, err := security.DecryptJSONFields(v); err == nil {
                        v = outJSON
                    } else {
                        // leave as-is on failure
                    }
                }
            } else {
                dec, err := security.Decrypt(v)
                if err != nil {
                    return nil, err
                }
                v = dec
            }
        }
        out = append(out, string(v))
    }
    return out, iter.Error()
}
