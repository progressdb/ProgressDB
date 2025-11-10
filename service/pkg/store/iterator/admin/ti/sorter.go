package ti

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

// SortThreads sorts threads by specified field
func (ts *ThreadSorter) SortThreads(threads []models.Thread, sortBy string) []models.Thread {
	if len(threads) == 0 {
		return threads
	}

	if sortBy == "" {
		sortBy = "created_ts"
	}

	switch sortBy {
	case "created_ts":
		ts.sortByCreatedTS(threads)
	case "updated_ts":
		ts.sortByUpdatedTS(threads)
	default:
		ts.sortByCreatedTS(threads)
	}

	return threads
}

func (ts *ThreadSorter) sortByCreatedTS(threads []models.Thread) {
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].CreatedTS < threads[j].CreatedTS
	})
}

func (ts *ThreadSorter) sortByUpdatedTS(threads []models.Thread) {
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].UpdatedTS < threads[j].UpdatedTS
	})
}
