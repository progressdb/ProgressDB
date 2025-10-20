package threads

import (
	"bytes"
	"fmt"
	"strings"

	"progressdb/pkg/store/db"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// lists all saved thread metadata as JSON
func ListThreads() ([]string, error) {
	tr := telemetry.Track("store.list_threads")
	defer tr.Finish()

	if db.StoreDB == nil {
		return nil, fmt.Errorf("pebble not opened; call store.Open first")
	}
	prefix := []byte("thread:")
	tr.Mark("new_iter")
	iter, err := db.StoreDB.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	tr.Mark("iterate")
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := string(iter.Key())
		if strings.HasSuffix(k, ":meta") {
			v := append([]byte(nil), iter.Value()...)
			out = append(out, string(v))
		}
	}
	return out, iter.Error()
}
