package store

import (
	"fmt"

	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// gets thread metadata JSON for id
func GetThread(threadID string) (string, error) {
	tr := telemetry.Track("store.get_thread")
	defer tr.Finish()

	if db == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := ThreadMetaKey(threadID)
	if err != nil {
		return "", fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("get")
	v, closer, err := db.Get([]byte(tk))
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	return string(v), nil
}
