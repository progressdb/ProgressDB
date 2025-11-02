package thread

import (
	"sort"

	"progressdb/pkg/models"
)

// ThreadSorter handles sorting threads by different fields
type ThreadSorter struct{}

// NewThreadSorter creates a new thread sorter
func NewThreadSorter() *ThreadSorter {
	return &ThreadSorter{}
}

// SortThreads sorts threads by specified field and order
func (ts *ThreadSorter) SortThreads(threads []models.Thread, sortBy, orderBy string) []models.Thread {
	if len(threads) == 0 {
		return threads
	}

	// Default sort field and order
	if sortBy == "" {
		sortBy = "created_at"
	}
	if orderBy == "" {
		orderBy = "desc"
	}

	switch sortBy {
	case "created_at", "created_ts":
		ts.sortByCreatedTS(threads, orderBy)
	case "updated_at", "updated_ts":
		ts.sortByUpdatedTS(threads, orderBy)
	default:
		// Default to created_ts if unknown field
		ts.sortByCreatedTS(threads, orderBy)
	}

	return threads
}

// sortByCreatedTS sorts threads by creation timestamp
func (ts *ThreadSorter) sortByCreatedTS(threads []models.Thread, orderBy string) {
	sort.Slice(threads, func(i, j int) bool {
		tsI := threads[i].CreatedTS
		tsJ := threads[j].CreatedTS

		if orderBy == "desc" {
			return tsI > tsJ
		}
		return tsI < tsJ
	})
}

// sortByUpdatedTS sorts threads by update timestamp
func (ts *ThreadSorter) sortByUpdatedTS(threads []models.Thread, orderBy string) {
	sort.Slice(threads, func(i, j int) bool {
		tsI := threads[i].UpdatedTS
		tsJ := threads[j].UpdatedTS

		if orderBy == "desc" {
			return tsI > tsJ
		}
		return tsI < tsJ
	})
}
