package ki

import (
	"sort"

	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type KeySorter struct{}

func NewKeySorter() *KeySorter {
	return &KeySorter{}
}

// extractTimestampFromKey extracts timestamp from key using proper parsing utilities
func (ks *KeySorter) extractTimestampFromKey(key string, sortBy string) int64 {
	// Parse key using proper utility to identify type
	parsed, err := keys.ParseKey(key)
	if err != nil {
		return 0
	}

	// Handle different key types
	switch parsed.Type {
	case keys.KeyTypeMessage:
		// Extract timestamp from message key
		return ks.extractMessageTimestamp(parsed, sortBy)
	case keys.KeyTypeThread:
		// Extract timestamp from thread key
		return ks.extractThreadTimestamp(parsed, sortBy)
	default:
		// For other key types, try to extract from thread timestamp as fallback
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	}

	return 0
}

// extractMessageTimestamp extracts timestamp from message key parts
func (ks *KeySorter) extractMessageTimestamp(parsed *keys.KeyParts, sortBy string) int64 {
	// Extract timestamp based on sort field
	switch sortBy {
	case "created_at", "created_ts":
		// Parse message timestamp from MessageKey
		if ts, err := keys.ParseKeyTimestamp(parsed.MessageTS); err == nil {
			return ts
		}
	case "updated_at", "updated_ts":
		// For messages, updated_ts is same as created_ts in the key
		if ts, err := keys.ParseKeyTimestamp(parsed.MessageTS); err == nil {
			return ts
		}
	default:
		// Default to created_ts
		if ts, err := keys.ParseKeyTimestamp(parsed.MessageTS); err == nil {
			return ts
		}
	}

	return 0
}

// extractThreadTimestamp extracts timestamp from thread key parts
func (ks *KeySorter) extractThreadTimestamp(parsed *keys.KeyParts, sortBy string) int64 {
	// Extract timestamp based on sort field
	switch sortBy {
	case "created_at", "created_ts":
		// Parse thread timestamp
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	case "updated_at", "updated_ts":
		// For threads, updated_ts is same as created_ts in the key
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	default:
		// Default to created_ts
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	}

	return 0
}

func (ks *KeySorter) SortKeys(keys []string, sortBy, orderBy string, response *pagination.PaginationResponse) []string {
	if len(keys) == 0 {
		return keys
	}

	// Default values
	if sortBy == "" {
		sortBy = "created_at"
	}
	if orderBy == "" {
		orderBy = "asc"
	}

	// Sort by timestamp first
	sort.Slice(keys, func(i, j int) bool {
		tsI := ks.extractTimestampFromKey(keys[i], sortBy)
		tsJ := ks.extractTimestampFromKey(keys[j], sortBy)

		if orderBy == "desc" {
			// Reverse order: newest first → oldest last
			return tsI > tsJ
		}
		// Chat-style: oldest first → newest last
		return tsI < tsJ
	})

	// Update response
	response.OrderBy = orderBy

	// Update anchors based on sorted order
	if len(keys) > 0 {
		response.StartAnchor = keys[0]
		response.EndAnchor = keys[len(keys)-1]
	}

	return keys
}
