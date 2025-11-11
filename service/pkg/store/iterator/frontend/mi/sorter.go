package mi

import (
	"sort"

	"progressdb/pkg/models"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type MessageSorter struct{}

func NewMessageSorter() *MessageSorter {
	return &MessageSorter{}
}

func (ms *MessageSorter) SortMessages(messages []models.Message, sortBy string) []models.Message {
	if len(messages) == 0 {
		return messages
	}

	if sortBy == "" {
		sortBy = "created_ts"
	}

	switch sortBy {
	case "created_ts":
		ms.sortByCreatedTS(messages)
	case "updated_ts":
		ms.sortByUpdatedTS(messages)
	default:
		ms.sortByCreatedTS(messages)
	}

	return messages
}

func (ms *MessageSorter) SortKeys(keys []string, sortBy string, response *pagination.PaginationResponse) []string {
	if len(keys) == 0 {
		return keys
	}

	if sortBy == "" {
		sortBy = "created_ts"
	}

	sort.Slice(keys, func(i, j int) bool {
		tsI := ms.extractTimestampFromKey(keys[i], sortBy)
		tsJ := ms.extractTimestampFromKey(keys[j], sortBy)
		return tsI < tsJ // Ascending order for key iteration
	})

	// Anchors will be set by main iterator logic

	return keys
}

func (ms *MessageSorter) extractTimestampFromKey(key string, sortBy string) int64 {
	parsed, err := keys.ParseKey(key)
	if err != nil {
		return 0
	}

	switch parsed.Type {
	case keys.KeyTypeMessage:
		return ms.extractMessageTimestamp(parsed, sortBy)
	case keys.KeyTypeThread:
		return ms.extractThreadTimestamp(parsed, sortBy)
	default:
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	}

	return 0
}

func (ms *MessageSorter) extractMessageTimestamp(parsed *keys.KeyParts, sortBy string) int64 {
	switch sortBy {
	case "created_at", "created_ts":
		if ts, err := keys.ParseKeyTimestamp(parsed.MessageTS); err == nil {
			return ts
		}
	case "updated_at", "updated_ts":
		if ts, err := keys.ParseKeyTimestamp(parsed.MessageTS); err == nil {
			return ts
		}
	default:
		if ts, err := keys.ParseKeyTimestamp(parsed.MessageTS); err == nil {
			return ts
		}
	}

	return 0
}

func (ms *MessageSorter) extractSequenceFromKey(key string) uint64 {
	parsed, err := keys.ParseKey(key)
	if err != nil {
		return 0
	}
	if parsed.Type == keys.KeyTypeMessage {
		if seq, err := keys.ParseKeySequence(parsed.Seq); err == nil {
			return seq
		}
	}
	return 0
}

func (ms *MessageSorter) extractThreadTimestamp(parsed *keys.KeyParts, sortBy string) int64 {
	switch sortBy {
	case "created_at", "created_ts":
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	case "updated_at", "updated_ts":
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	default:
		if ts, err := keys.ParseKeyTimestamp(parsed.ThreadTS); err == nil {
			return ts
		}
	}

	return 0
}

func (ms *MessageSorter) sortByCreatedTS(messages []models.Message) {
	sort.Slice(messages, func(i, j int) bool {
		tsI := messages[i].CreatedTS
		tsJ := messages[j].CreatedTS
		if tsI != tsJ {
			return tsI < tsJ // Primary sort by timestamp
		}
		// Tiebreaker: use sequence from message key
		seqI := ms.extractSequenceFromKey(messages[i].Key)
		seqJ := ms.extractSequenceFromKey(messages[j].Key)
		return seqI < seqJ
	})
}

func (ms *MessageSorter) sortByUpdatedTS(messages []models.Message) {
	sort.Slice(messages, func(i, j int) bool {
		tsI := messages[i].UpdatedTS
		tsJ := messages[j].UpdatedTS
		if tsI != tsJ {
			return tsI < tsJ // Primary sort by timestamp
		}
		// Tiebreaker: use sequence from message key
		seqI := ms.extractSequenceFromKey(messages[i].Key)
		seqJ := ms.extractSequenceFromKey(messages[j].Key)
		return seqI < seqJ
	})
}
