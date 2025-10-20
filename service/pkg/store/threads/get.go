package threads

import (
	"fmt"

	"progressdb/pkg/store/db"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// gets thread metadata JSON for id
func GetThread(threadID string) (string, error) {
	tr := telemetry.Track("store.get_thread")
	defer tr.Finish()

	if db.StoreDB == nil {
		return "", fmt.Errorf("pebble not opened; call store.Open first")
	}
	tk, err := keys.ThreadMetaKey(threadID)
	if err != nil {
		return "", fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("get")
	v, closer, err := db.StoreDB.Get([]byte(tk))
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	return string(v), nil
}
