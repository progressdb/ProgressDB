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

// fetchAnchorWindowKeys gets a window of valid keys around the anchor (including anchor in middle)
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
	candidates, err := km.getKeyCandidatesBefore(prefix, reference, limit*2)
	if err != nil {
		return nil, err
	}

	validKeys := make([]string, 0, limit)
	for _, key := range candidates {
		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			continue
		}
		if isDeleted(parsed.ThreadKey) {
			continue
		}
		validKeys = append(validKeys, key)
		if len(validKeys) >= limit {
			break
		}
	}

	return validKeys, nil
}

func (km *KeyManager) fetchAfterKeys(prefix, reference string, limit int, isDeleted func(string) bool) ([]string, error) {
	candidates, err := km.getKeyCandidatesAfter(prefix, reference, limit*2)
	if err != nil {
		return nil, err
	}

	validKeys := make([]string, 0, limit)
	for _, key := range candidates {
		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			continue
		}
		if isDeleted(parsed.ThreadKey) {
			continue
		}
		validKeys = append(validKeys, key)
		if len(validKeys) >= limit {
			break
		}
	}

	return validKeys, nil
}

func (km *KeyManager) fetchInitialLoadKeys(prefix string, limit int, isDeleted func(string) bool) ([]string, error) {
	candidates, err := km.getKeyCandidatesInitial(prefix, limit*2)
	if err != nil {
		return nil, err
	}

	validKeys := make([]string, 0, limit)
	for _, key := range candidates {
		parsed, err := keys.ParseUserOwnsThread(key)
		if err != nil {
			continue
		}
		if isDeleted(parsed.ThreadKey) {
			continue
		}
		validKeys = append(validKeys, key)
		if len(validKeys) >= limit {
			break
		}
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

func (km *KeyManager) getKeyCandidatesBefore(prefix, reference string, maxCandidates int) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	candidates := make([]string, 0, maxCandidates)

	for valid && len(candidates) < maxCandidates {
		candidates = append(candidates, string(iter.Key()))
		valid = iter.Prev()
	}

	return candidates, nil
}

func (km *KeyManager) getKeyCandidatesAfter(prefix, reference string, maxCandidates int) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	if valid && string(iter.Key()) == reference {
		valid = iter.Next()
	}

	candidates := make([]string, 0, maxCandidates)
	for valid && len(candidates) < maxCandidates {
		candidates = append(candidates, string(iter.Key()))
		valid = iter.Next()
	}

	return candidates, nil
}

func (km *KeyManager) getKeyCandidatesInitial(prefix string, maxCandidates int) ([]string, error) {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	valid := iter.First()
	candidates := make([]string, 0, maxCandidates)

	for valid && len(candidates) < maxCandidates {
		candidates = append(candidates, string(iter.Key()))
		valid = iter.Next()
	}

	return candidates, nil
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

// checkHasKeysBefore checks if there are valid (non-deleted) keys before the reference
func (km *KeyManager) checkHasKeysBefore(prefix, reference string) bool {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return false
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	if !valid {
		return false
	}

	// Move to just before reference
	if string(iter.Key()) == reference {
		valid = iter.Prev()
	}

	isDeleted := func(threadKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil
	}

	// Check if there's at least one valid key before
	for valid {
		key := string(iter.Key())
		parsed, err := keys.ParseUserOwnsThread(key)
		if err == nil && !isDeleted(parsed.ThreadKey) {
			return true
		}
		valid = iter.Prev()
	}

	return false
}

// checkHasKeysAfter checks if there are valid (non-deleted) keys after the reference
func (km *KeyManager) checkHasKeysAfter(prefix, reference string) bool {
	iter, err := km.createIterator(prefix)
	if err != nil {
		return false
	}
	defer iter.Close()

	valid := iter.SeekGE([]byte(reference))
	if !valid {
		return false
	}

	// Skip the reference itself
	if string(iter.Key()) == reference {
		valid = iter.Next()
	}

	isDeleted := func(threadKey string) bool {
		deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
		_, err := indexdb.GetKey(deleteMarkerKey)
		return err == nil
	}

	// Check if there's at least one valid key after
	for valid {
		key := string(iter.Key())
		parsed, err := keys.ParseUserOwnsThread(key)
		if err == nil && !isDeleted(parsed.ThreadKey) {
			return true
		}
		valid = iter.Next()
	}

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
