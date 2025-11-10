package mi

import (
	"sort"
	"strconv"
	"strings"

	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

// MessageSorter handles sorting message keys by timestamp
type MessageSorter struct{}

// NewMessageSorter creates a new message sorter
func NewMessageSorter() *MessageSorter {
	return &MessageSorter{}
}

// extractTimestampFromKey extracts timestamp from message key
func (ms *MessageSorter) extractTimestampFromKey(key string) int64 {
	parsed, err := keys.ParseKey(key)
	if err != nil {
		return 0
	}

	switch parsed.Type {
	case keys.KeyTypeMessage:
		return ms.extractMessageTimestamp(parsed, "created_ts")
	default:
		// For other key types, try to extract from thread timestamp as fallback
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	}

	return 0
}

// extractSequenceFromKey extracts sequence number from message key
func (ms *MessageSorter) extractSequenceFromKey(key string) int64 {
	parsed, err := keys.ParseKey(key)
	if err != nil {
		return 0
	}

	switch parsed.Type {
	case keys.KeyTypeMessage:
		// Parse sequence from MessageKey (the last part after the final colon)
		if parsed.MessageKey != "" {
			// Extract sequence from the full message key
			parts := strings.Split(parsed.MessageKey, ":")
			if len(parts) >= 3 {
				if seq, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
					return seq
				}
			}
		}
	}

	return 0
}

// extractMessageTimestamp extracts timestamp from message key parts
func (ms *MessageSorter) extractMessageTimestamp(parsed *keys.KeyParts, sortBy string) int64 {
	// Extract timestamp based on sort field
	switch sortBy {
	case "created_ts", "created_at":
		// Parse message timestamp from MessageKey
		if ts, err := keys.ParseKeyTimestamp(parsed.MessageTS); err == nil {
			return ts
		}
	case "updated_ts", "updated_at":
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

// SortKeys sorts message keys by timestamp then sequence in ascending order (oldest→newest)
func (ms *MessageSorter) SortKeys(keys []string, sortBy string, response *pagination.PaginationResponse) []string {
	if len(keys) == 0 {
		return keys
	}

	// Sort by timestamp first, then sequence (oldest first for chat-style display)
	sort.Slice(keys, func(i, j int) bool {
		tsI := ms.extractTimestampFromKey(keys[i])
		tsJ := ms.extractTimestampFromKey(keys[j])

		if tsI != tsJ {
			return tsI < tsJ // Primary sort by timestamp
		}

		// If timestamps are equal, sort by sequence
		seqI := ms.extractSequenceFromKey(keys[i])
		seqJ := ms.extractSequenceFromKey(keys[j])
		return seqI < seqJ // Secondary sort by sequence
	})

	// Update response anchors based on sorted order (oldest→newest)
	if len(keys) > 0 {
		response.BeforeAnchor = keys[0]          // oldest
		response.AfterAnchor = keys[len(keys)-1] // newest
	}

	return keys
}
