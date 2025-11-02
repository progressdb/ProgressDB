package mi

import (
	"sort"

	"progressdb/pkg/models"
)

// MessageSorter handles sorting messages by different fields
type MessageSorter struct{}

// NewMessageSorter creates a new message sorter
func NewMessageSorter() *MessageSorter {
	return &MessageSorter{}
}

// SortMessages sorts messages by specified field and order
func (ms *MessageSorter) SortMessages(messages []models.Message, sortBy, orderBy string) []models.Message {
	if len(messages) == 0 {
		return messages
	}

	// Default sort field and order
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
		// Default to created_ts if unknown field
		ms.sortByCreatedTS(messages, orderBy)
	}

	return messages
}

// sortByCreatedTS sorts messages by creation timestamp
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

// sortByUpdatedTS sorts messages by update timestamp
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
