package ti

import (
	"fmt"
	"strings"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"

	"github.com/cockroachdb/pebble"
)

type KeyManager struct {
	db *pebble.DB
}

func NewKeyManager(db *pebble.DB) *KeyManager {
	return &KeyManager{db: db}
}

func (km *KeyManager) ExecuteKeyQuery(userID, prefix string, req pagination.PaginationRequest) ([]string, error) {
	isDeleted := func(threadKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil // Marker exists = thread deleted
	}
	logger.Debug("[KeyManager] Query",
		"userID", userID,
		"prefix", prefix,
		"before", req.Before,
		"after", req.After,
		"anchor", req.Anchor,
		"limit", req.Limit,
	)

	var resultKeys []string

	switch {
	case req.Anchor != "":
		keys, err := km.fetchAnchorWindowKeys(userID, prefix, req, isDeleted)
		if err != nil {
			return nil, err
		}
		resultKeys = keys

	case req.Before != "":
		keys, err := km.fetchBeforeKeys(prefix, req.Before, req.Limit, isDeleted)
		if err != nil {
			return nil, err
		}
		resultKeys = keys

	case req.After != "":
		keys, err := km.fetchAfterKeys(prefix, req.After, req.Limit, isDeleted)
		if err != nil {
			return nil, err
		}
		resultKeys = keys

	default:
		keys, err := km.fetchInitialLoadKeys(prefix, req.Limit, isDeleted)
		if err != nil {
			return nil, err
		}
		resultKeys = keys
	}

	logger.Debug("[KeyManager] Returned keys", "count", len(resultKeys))
	return resultKeys, nil
}

func (km *KeyManager) fetchAnchorWindowKeys(userID, prefix string, req pagination.PaginationRequest, isDeleted func(string) bool) ([]string, error) {
	var anchorRelKey string
	if req.Anchor != "" {
		if strings.HasPrefix(req.Anchor, "rel:u:") {
			anchorRelKey = req.Anchor
		} else if strings.HasPrefix(req.Anchor, "t:") {
			parsed, err := keys.ParseKey(req.Anchor)
			if err != nil || parsed.Type != keys.KeyTypeThread {
				return nil, fmt.Errorf("invalid anchor thread key: %s", req.Anchor)
			}
			anchorRelKey = fmt.Sprintf(keys.RelUserOwnsThread, userID, parsed.ThreadTS)
		} else {
			return nil, fmt.Errorf("invalid anchor format: %s", req.Anchor)
		}
	}

	logger.Debug("[fetchAnchorWindowKeys] Starting", "anchorRelKey", anchorRelKey, "limit", req.Limit)

	beforeLimit := req.Limit
	afterLimit := req.Limit

	logger.Debug("[fetchAnchorWindowKeys] Distribution", "beforeLimit", beforeLimit, "afterLimit", afterLimit)

	beforeKeys, err := km.getKeysBeforeAnchor(prefix, anchorRelKey, beforeLimit, isDeleted)
	if err != nil {
		return nil, err
	}

	afterKeys, err := km.getKeysAfterAnchor(prefix, anchorRelKey, afterLimit, isDeleted)
	if err != nil {
		return nil, err
	}

	// Combine before + anchor (if not deleted) + after
	resultKeys := beforeKeys
	if parsed, err := keys.ParseUserOwnsThread(anchorRelKey); err == nil && !isDeleted(parsed.ThreadKey) {
		resultKeys = append(resultKeys, anchorRelKey)
	}
	resultKeys = append(resultKeys, afterKeys...)

	logger.Debug("[fetchAnchorWindowKeys] Combined", "beforeKeys", len(beforeKeys), "afterKeys", len(afterKeys), "anchorIncluded", "total", len(resultKeys))

	logger.Debug("[fetchAnchorWindowKeys] Result", "beforeKeys", len(beforeKeys), "afterKeys", len(afterKeys), "resultKeys", len(resultKeys))
	return resultKeys, nil
}

func (km *KeyManager) fetchBeforeKeys(prefix, reference string, limit int, isDeleted func(string) bool) ([]string, error) {
	iter, err := km.createIterator(prefix)
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

		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			valid = iter.Prev()
			continue
		}

		if !isDeleted(parsed.ThreadKey) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Prev()
	}

	// Reverse to maintain correct order
	for i, j := 0, len(validKeys)-1; i < j; i, j = i+1, j-1 {
		validKeys[i], validKeys[j] = validKeys[j], validKeys[i]
	}

	return validKeys, nil
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

		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			valid = iter.Next()
			continue
		}

		if !isDeleted(parsed.ThreadKey) {
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

	valid := iter.First()

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			valid = iter.Next()
			continue
		}

		if !isDeleted(parsed.ThreadKey) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Next()
	}

	return validKeys, nil
}

func (km *KeyManager) getKeysBeforeAnchor(prefix, anchorRelKey string, limit int, isDeleted func(string) bool) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(anchorRelKey))
	if !valid {
		return nil, nil
	}

	if string(iter.Key()) == anchorRelKey {
		valid = iter.Prev()
	}

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			valid = iter.Prev()
			continue
		}

		if !isDeleted(parsed.ThreadKey) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Prev()
	}

	for i, j := 0, len(validKeys)-1; i < j; i, j = i+1, j-1 {
		validKeys[i], validKeys[j] = validKeys[j], validKeys[i]
	}

	logger.Debug("[getKeysBeforeAnchor]", "anchor", anchorRelKey, "limit", limit, "found", len(validKeys))
	return validKeys, nil
}

func (km *KeyManager) getKeysAfterAnchor(prefix, anchorRelKey string, limit int, isDeleted func(string) bool) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(anchorRelKey))
	if !valid {
		return nil, nil
	}

	if string(iter.Key()) == anchorRelKey {
		valid = iter.Next()
	}

	validKeys := make([]string, 0, limit)
	for valid && len(validKeys) < limit {
		key := string(iter.Key())

		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			valid = iter.Next()
			continue
		}

		if !isDeleted(parsed.ThreadKey) {
			validKeys = append(validKeys, key)
		}

		valid = iter.Next()
	}

	logger.Debug("[getKeysAfterAnchor]", "anchor", anchorRelKey, "limit", limit, "found", len(validKeys))
	return validKeys, nil
}

func (km *KeyManager) createIterator(prefix string) (*pebble.Iterator, error) {
	if prefix == "" {
		return km.db.NewIter(&pebble.IterOptions{})
	}
	lowerBound := []byte(prefix)
	upperBound := nextPrefix([]byte(prefix))
	return km.db.NewIter(&pebble.IterOptions{
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

	// Move to just before reference
	if string(iter.Key()) == reference {
		logger.Debug("[checkHasKeysBefore] Skipping reference", "reference", reference)
		valid = iter.Prev()
	}

	isDeleted := func(threadKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil
	}

	checks := 0
	for valid {
		key := string(iter.Key())
		parsed, err := keys.ParseUserOwnsThread(key)
		logger.Debug("[checkHasKeysBefore] Iter", "key", key, "parseErr", err)
		if err == nil && !isDeleted(parsed.ThreadKey) {
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

	isDeleted := func(threadKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil
	}

	checks := 0
	for valid {
		key := string(iter.Key())
		parsed, err := keys.ParseUserOwnsThread(key)
		logger.Debug("[checkHasKeysAfter] Iter", "key", key, "parseErr", err)
		if err == nil && !isDeleted(parsed.ThreadKey) {
			logger.Debug("[checkHasKeysAfter] Found key after", "key", key)
			return true
		}
		valid = iter.Next()
		checks++
	}
	logger.Debug("[checkHasKeysAfter] No key found after", "reference", reference, "iterations", checks)
	return false
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
