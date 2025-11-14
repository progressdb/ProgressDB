package tracking

import (
	"fmt"
	"sync"

	"progressdb/pkg/state/logger"
	"progressdb/pkg/store/db/storedb"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/keys"
)

var GlobalKeyMapper *KeyMapper

type KeyMapper struct {
	tracker    *InflightTracker
	batchCache map[string]string // provisionalKey -> finalKey for current batch
	mu         sync.RWMutex
}

func NewKeyMapper(tracker *InflightTracker) *KeyMapper {
	return &KeyMapper{
		tracker:    tracker,
		batchCache: make(map[string]string),
	}
}

func InitGlobalKeyMapper() {
	GlobalKeyMapper = NewKeyMapper(GlobalInflightTracker)
}

// ResolveKey resolves a key to its final form, handling all lifecycle states
// Returns: (finalKey, found, error)
func (km *KeyMapper) ResolveKey(key string) (string, bool, error) {
	if key == "" {
		return "", false, fmt.Errorf("key cannot be empty")
	}

	// 1. Already a final (non-provisional) key? Just return it.
	if parsed, err := keys.ParseKey(key); err == nil && parsed.Type != keys.KeyTypeMessageProvisional {
		return key, true, nil
	}

	// 2. Check batch cache first (fastest)
	if finalKey, ok := km.getFromBatchCache(key); ok {
		logger.Debug("resolve_key", "source", "batch_cache", "provisional", key, "final", finalKey)
		return finalKey, true, nil
	}

	// 3. Check if key is in-flight and wait for completion
	if km.tracker.IsInflight(key) {
		logger.Debug("resolve_key", "source", "inflight_wait", "key", key)
		km.tracker.WaitForInflight(key)
		// After waiting, try database lookup (key should be committed now)
		return km.resolveFromDatabase(key)
	}

	// 4. Try database lookup
	return km.resolveFromDatabase(key)
}

// ResolveKeyOrWait resolves a key and waits if it's in-flight
// Convenience method that returns only finalKey or error
func (km *KeyMapper) ResolveKeyOrWait(key string) (string, error) {
	finalKey, found, err := km.ResolveKey(key)
	if err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("key not found: %s", key)
	}
	return finalKey, nil
}

// PrepopulateBatchCache populates the batch cache with provisional->final mappings
// Called by apply workers when processing a batch
func (km *KeyMapper) PrepopulateBatchCache(mappings map[string]string) {
	km.mu.Lock()
	defer km.mu.Unlock()

	for provKey, finalKey := range mappings {
		km.batchCache[provKey] = finalKey
		logger.Debug("batch_cache_populated", "provisional", provKey, "final", finalKey)
	}

	logger.Debug("batch_cache_prepopulated", "mappings_count", len(mappings))
}

// ClearBatchCache clears the batch cache
// Called after batch processing is complete
func (km *KeyMapper) ClearBatchCache() {
	km.mu.Lock()
	defer km.mu.Unlock()

	km.batchCache = make(map[string]string)
	logger.Debug("batch_cache_cleared")
}

// getFromBatchCache gets a mapping from batch cache (thread-safe)
func (km *KeyMapper) getFromBatchCache(provisionalKey string) (string, bool) {
	km.mu.RLock()
	defer km.mu.RUnlock()

	finalKey, exists := km.batchCache[provisionalKey]
	return finalKey, exists
}

// resolveFromDatabase looks up the final key in the database
// This is the existing logic from apply/sequence.go
func (km *KeyMapper) resolveFromDatabase(provisionalKey string) (string, bool, error) {
	// For message keys, search for the sequenced final key
	if keys.IsProvisionalMessageKey(provisionalKey) {
		if storedb.Client == nil {
			return "", false, nil
		}
		prefix := provisionalKey + ":"

		iter, err := storedb.Client.NewIter(nil)
		if err != nil {
			return "", false, fmt.Errorf("failed to create iterator: %w", err)
		}
		defer iter.Close()

		iter.SeekGE([]byte(prefix))
		if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
			finalKey := string(iter.Key())
			logger.Debug("resolve_key", "source", "database", "provisional", provisionalKey, "final", finalKey)
			return finalKey, true, nil
		}
	}

	// For thread keys, check if thread exists in database
	// Thread keys don't change format, so just check existence
	if _, err := thread_store.GetThreadData(provisionalKey); err == nil {
		logger.Debug("resolve_key", "source", "database_thread", "key", provisionalKey)
		return provisionalKey, true, nil
	}

	logger.Debug("resolve_key", "source", "not_found", "key", provisionalKey)
	return "", false, nil
}
