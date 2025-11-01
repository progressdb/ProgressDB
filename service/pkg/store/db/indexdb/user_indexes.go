package indexdb

import (
	"fmt"
	"strings"

	"progressdb/pkg/store/keys"
)

func ListUserThreadKeys(userID string) ([]string, error) {
	prefix, err := keys.GenUserThreadRelPrefix(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to generate user thread prefix: %w", err)
	}
	iter, err := DBIter()
	if err != nil {
		return nil, fmt.Errorf("failed to create DB iterator: %w", err)
	}
	defer iter.Close()
	var threadKeys []string

	seekKey := []byte(prefix)
	for ok := iter.SeekGE(seekKey); ok && iter.Valid(); ok = iter.Next() {
		key := string(iter.Key())
		if !strings.HasPrefix(key, prefix) {
			break
		}
		threadKeys = append(threadKeys, key)
	}
	return threadKeys, nil
}

type ThreadWithTimestamp struct {
	Key       string
	Timestamp int64
}
