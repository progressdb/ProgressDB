package threads

import (
	"fmt"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

// gets thread metadata JSON for id
func GetThread(threadID string) (string, error) {
	if storedb.Client == nil {
		return "", fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	tk := keys.GenThreadKey(threadID)

	v, closer, err := storedb.Client.Get([]byte(tk))
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	return string(v), nil
}

func CheckThreadExists(threadID string) (bool, error) {
	if storedb.Client == nil {
		return false, fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	tk := keys.GenThreadKey(threadID)

	_, closer, err := storedb.Client.Get([]byte(tk))
	if closer != nil {
		defer closer.Close()
	}
	if err != nil {
		// not found
		return false, nil
	}
	return true, nil
}
