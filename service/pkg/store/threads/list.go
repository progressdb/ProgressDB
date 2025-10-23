package threads

import (
	"bytes"
	"fmt"
	"strings"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// lists all saved thread metadata as JSON
func ListThreads() ([]string, error) {
	tr := telemetry.Track("storedb.list_threads")
	defer tr.Finish()

	if storedb.Client == nil {
		return nil, fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	prefix := []byte(keys.GenThreadPrefix())
	tr.Mark("new_iter")
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
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
