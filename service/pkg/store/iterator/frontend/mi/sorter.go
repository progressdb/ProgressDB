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

func (ms *MessageSorter) SortMessages(messages []models.Message, sortBy, orderBy string) []models.Message {
	if len(messages) == 0 {
		return messages
	}

	if sortBy == "" {
		sortBy = "created_at"
	}
	if orderBy == "" {
		orderBy = "asc"
	}

	switch sortBy {
	case "created_at", "created_ts":
		ms.sortByCreatedTS(messages, orderBy)
	case "updated_at", "updated_ts":
		ms.sortByUpdatedTS(messages, orderBy)
	default:
		ms.sortByCreatedTS(messages, orderBy)
	}

	return messages
}

func (ms *MessageSorter) SortKeys(keys []string, sortBy, orderBy string, response *pagination.PaginationResponse) []string {
	if len(keys) == 0 {
		return keys
	}

	if sortBy == "" {
		sortBy = "created_at"
	}
	if orderBy == "" {
		orderBy = "asc"
	}

	sort.Slice(keys, func(i, j int) bool {
		tsI := ms.extractTimestampFromKey(keys[i], sortBy)
		tsJ := ms.extractTimestampFromKey(keys[j], sortBy)

		if orderBy == "desc" {
			return tsI > tsJ
		}
		return tsI < tsJ
	})

	response.OrderBy = orderBy

	if len(keys) > 0 {
		response.StartAnchor = keys[0]
		response.EndAnchor = keys[len(keys)-1]
	}

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

func (ms *MessageSorter) sortByCreatedTS(messages []models.Message, orderBy string) {
	sort.Slice(messages, func(i, j int) bool {
		tsI := messages[i].CreatedTS
		tsJ := messages[j].CreatedTS

		if orderBy == "desc" {
			return tsI > tsJ
		}
		return tsI < tsJ
	})
}

func (ms *MessageSorter) sortByUpdatedTS(messages []models.Message, orderBy string) {
	sort.Slice(messages, func(i, j int) bool {
		tsI := messages[i].UpdatedTS
		tsJ := messages[j].UpdatedTS

		if orderBy == "desc" {
			return tsI > tsJ
		}
		return tsI < tsJ
	})
}
