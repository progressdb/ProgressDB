package threads

import (
	"fmt"

	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
)

// gets thread metadata JSON for id
func GetThreadData(threadKey string) (string, error) {
	if storedb.Client == nil {
		return "", fmt.Errorf("pebble not opened; call storedb.Open first")
	}

	v, closer, err := storedb.Client.Get([]byte(threadKey))
	if err != nil {
		return "", err
	}
	if closer != nil {
		defer closer.Close()
	}
	return string(v), nil
}

func CheckThreadExists(threadKey string) (bool, error) {
	if storedb.Client == nil {
		return false, fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	tk := keys.GenThreadKey(threadKey)

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
