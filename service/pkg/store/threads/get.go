package threads

import (
	"fmt"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"
)

// gets thread metadata JSON for id
func GetThread(threadID string) (string, error) {
	tr := telemetry.Track("storedb.get_thread")
	defer tr.Finish()

	if storedb.Client == nil {
		return "", fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	tk, err := keys.ThreadMetaKey(threadID)
	if err != nil {
		return "", fmt.Errorf("invalid thread id: %w", err)
	}
	tr.Mark("get")
	v, closer, err := storedb.Client.Get([]byte(tk))
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	return string(v), nil
}
