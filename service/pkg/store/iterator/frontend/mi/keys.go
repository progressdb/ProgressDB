package mi

import (
	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

type KeyManager struct {
	// No DB needed - using StoreDB directly
}

func NewKeyManager() *KeyManager {
	return &KeyManager{}
}

func (km *KeyManager) ExecuteKeyQuery(threadKey, prefix string, req pagination.PaginationRequest) ([]string, error) {
	isDeleted := func(messageKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil // Marker exists = message deleted
	}

	logger.Debug("[MI KeyManager] Query",
		"threadKey", threadKey,
		"prefix", prefix,
		"before", req.Before,
		"after", req.After,
		"anchor", req.Anchor,
		"limit", req.Limit,
	)

	var resultKeys []string

	switch {
	case req.Anchor != "":
		keys, err := km.fetchAnchorWindowKeys(prefix, req, isDeleted)
		if err != nil {
			return nil, err
		}
		resultKeys = keys

	case req.Before != "":
		keys, err := km.fetchBeforeKeys(prefix, req.Before, req.Limit, isDeleted)
		if err != nil {
			return nil, err
		}
		logger.Debug("[MI KeyManager] fetchBeforeKeys result", "keys", keys)
		resultKeys = keys

	case req.After != "":
		keys, err := km.fetchAfterKeys(prefix, req.After, req.Limit, isDeleted)
		if err != nil {
			return nil, err
		}
		resultKeys = keys
		logger.Debug("[MI KeyManager] fetchAfterKeys result", "keys", keys)

	default:
		keys, err := km.fetchInitialLoadKeys(prefix, req.Limit, isDeleted)
		if err != nil {
			return nil, err
		}
		resultKeys = keys
	}

	logger.Debug("[MI KeyManager] Returned keys", "count", len(resultKeys))
	return resultKeys, nil
}

func (km *KeyManager) fetchAnchorWindowKeys(prefix string, req pagination.PaginationRequest, isDeleted func(string) bool) ([]string, error) {
	anchorKey := req.Anchor

	logger.Debug("[fetchAnchorWindowKeys] Starting", "anchorKey", anchorKey, "limit", req.Limit)

	beforeLimit := req.Limit
	afterLimit := req.Limit

	logger.Debug("[fetchAnchorWindowKeys] Distribution", "beforeLimit", beforeLimit, "afterLimit", afterLimit)

	beforeKeys, err := km.getKeysBeforeAnchor(prefix, anchorKey, beforeLimit, isDeleted)
	if err != nil {
		return nil, err
	}

	afterKeys, err := km.getKeysAfterAnchor(prefix, anchorKey, afterLimit, isDeleted)
	if err != nil {
		return nil, err
	}

	// Combine before + anchor (if not deleted) + after
	resultKeys := beforeKeys
	if !isDeleted(anchorKey) {
		resultKeys = append(resultKeys, anchorKey)
	}
	resultKeys = append(resultKeys, afterKeys...)

	logger.Debug("[fetchAnchorWindowKeys] Combined", "beforeKeys", len(beforeKeys), "afterKeys", len(afterKeys), "anchorIncluded", "total", len(resultKeys))

	return resultKeys, nil
}

func (km *KeyManager) fetchBeforeKeys(prefix, reference string, limit int, isDeleted func(string) bool) ([]string, error) {
	// Use StoreDB iterator for message keys
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: []byte(prefix),
		UpperBound: nextPrefix([]byte(prefix)),
	})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	// Skip the reference key itself
	if valid && string(iter.Key()) == reference {
		valid = iter.Prev()
	}

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		if !isDeleted(key) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Prev()
	}

	return reverseKeys(validKeys), nil
}

func (km *KeyManager) fetchAfterKeys(prefix, reference string, limit int, isDeleted func(string) bool) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	// Skip the reference key itself
	if valid && string(iter.Key()) == reference {
		valid = iter.Next()
	}

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		if !isDeleted(key) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Next()
	}

	return validKeys, nil
}

func (km *KeyManager) fetchInitialLoadKeys(prefix string, limit int, isDeleted func(string) bool) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.Last() // Start from newest for messages

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		if !isDeleted(key) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Prev()
	}

	return reverseKeys(validKeys), nil
}

func (km *KeyManager) getKeysBeforeAnchor(prefix, anchorKey string, limit int, isDeleted func(string) bool) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(anchorKey))
	if !valid {
		return nil, nil
	}

	if string(iter.Key()) == anchorKey {
		valid = iter.Prev()
	}

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		if !isDeleted(key) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Prev()
	}

	logger.Debug("[getKeysBeforeAnchor]", "anchor", anchorKey, "limit", limit, "found", len(validKeys))
	return reverseKeys(validKeys), nil
}

func (km *KeyManager) getKeysAfterAnchor(prefix, anchorKey string, limit int, isDeleted func(string) bool) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(anchorKey))
	if !valid {
		return nil, nil
	}

	if string(iter.Key()) == anchorKey {
		valid = iter.Next()
	}

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		if !isDeleted(key) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Next()
	}

	logger.Debug("[getKeysAfterAnchor]", "anchor", anchorKey, "limit", limit, "found", len(validKeys))
	return validKeys, nil
}

func (km *KeyManager) createIterator(prefix string) (*pebble.Iterator, error) {
	if prefix == "" {
		return storedb.Client.NewIter(&pebble.IterOptions{})
	}
	lowerBound := []byte(prefix)
	upperBound := nextPrefix([]byte(prefix))
	return storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
}

func (km *KeyManager) checkHasKeysBefore(prefix, reference string) bool {
	logger.Debug("[checkHasKeysBefore] called", "prefix", prefix, "reference", reference)
	iter, err := km.createIterator(prefix)
	if err != nil {
		logger.Error("[checkHasKeysBefore] failed to create iterator", "prefix", prefix, "error", err)
		return false
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	if !valid {
		logger.Debug("[checkHasKeysBefore] SeekGE unsuccessful", "reference", reference)
		return false
	}

	// Move to just before reference (to older keys)
	if string(iter.Key()) == reference {
		logger.Debug("[checkHasKeysBefore] Skipping reference", "reference", reference)
		valid = iter.Prev()
	}

	isDeleted := func(messageKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil
	}

	checks := 0
	for valid {
		key := string(iter.Key())
		logger.Debug("[checkHasKeysBefore] Iter", "key", key)
		if !isDeleted(key) {
			logger.Debug("[checkHasKeysBefore] Found key before", "key", key)
			return true
		}
		valid = iter.Prev()
		checks++
	}
	logger.Debug("[checkHasKeysBefore] No key found before", "reference", reference, "iterations", checks)
	return false
}

func (km *KeyManager) checkHasKeysAfter(prefix, reference string) bool {
	logger.Debug("[checkHasKeysAfter] called", "prefix", prefix, "reference", reference)
	iter, err := km.createIterator(prefix)
	if err != nil {
		logger.Error("[checkHasKeysAfter] failed to create iterator", "prefix", prefix, "error", err)
		return false
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	if !valid {
		logger.Debug("[checkHasKeysAfter] SeekGE unsuccessful", "reference", reference)
		return false
	}

	// Skip the reference itself
	if string(iter.Key()) == reference {
		logger.Debug("[checkHasKeysAfter] Skipping reference", "reference", reference)
		valid = iter.Next()
	}

	isDeleted := func(messageKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil
	}

	checks := 0
	for valid {
		key := string(iter.Key())
		logger.Debug("[checkHasKeysAfter] Iter", "key", key)
		if !isDeleted(key) {
			logger.Debug("[checkHasKeysAfter] Found key after", "key", key)
			return true
		}
		valid = iter.Next()
		checks++
	}
	logger.Debug("[checkHasKeysAfter] No key found after", "reference", reference, "iterations", checks)
	return false
}

// reverses a slice of keys to handle database iteration direction
func reverseKeys(keys []string) []string {
	for i, j := 0, len(keys)-1; i < j; i, j = i+1, j-1 {
		keys[i], keys[j] = keys[j], keys[i]
	}
	return keys
}

func nextPrefix(prefix []byte) []byte {
	next := make([]byte, len(prefix))
	copy(next, prefix)
	for i := len(next) - 1; i >= 0; i-- {
		if next[i] < 0xff {
			next[i]++
			return next[:i+1]
		}
	}
	return append(next, 0x00)
}
