package ti

import (
	"sort"

	"progressdb/pkg/models"
)

type ThreadSorter struct{}

func NewThreadSorter() *ThreadSorter {
	return &ThreadSorter{}
}

func (ts *ThreadSorter) SortThreads(threads []models.Thread, sortBy string) []models.Thread {
	if len(threads) == 0 {
		return threads
	}

	if sortBy == "" {
		sortBy = "created_ts"
	}

	switch sortBy {
	case "created_ts", "created_at":
		ts.sortByCreatedTS(threads)
	case "updated_ts", "updated_at":
		ts.sortByUpdatedTS(threads)
	default:
		ts.sortByCreatedTS(threads)
	}

	return threads
}

func (ts *ThreadSorter) sortByCreatedTS(threads []models.Thread) {
	sort.Slice(threads, func(i, j int) bool {
		tsI := threads[i].CreatedTS
		tsJ := threads[j].CreatedTS
		return tsI > tsJ // Descending order (newest first)
	})
}

func (ts *ThreadSorter) sortByUpdatedTS(threads []models.Thread) {
	sort.Slice(threads, func(i, j int) bool {
		tsI := threads[i].UpdatedTS
		tsJ := threads[j].UpdatedTS
		return tsI > tsJ // Descending order (newest first)
	})
}
