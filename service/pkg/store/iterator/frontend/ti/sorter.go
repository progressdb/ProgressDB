package ti

import (
	"sort"

	"progressdb/pkg/models"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
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
	case "created_ts":
		ts.sortByCreatedTS(threads)
	case "updated_ts":
		ts.sortByUpdatedTS(threads)
	default:
		ts.sortByCreatedTS(threads)
	}

	return threads
}

func (ts *ThreadSorter) SortKeys(keys []string, sortBy string, response *pagination.PaginationResponse) []string {
	if len(keys) == 0 {
		return keys
	}

	if sortBy == "" {
		sortBy = "created_ts"
	}

	sort.Slice(keys, func(i, j int) bool {
		tsI := ts.extractTimestampFromKey(keys[i], sortBy)
		tsJ := ts.extractTimestampFromKey(keys[j], sortBy)
		return tsI < tsJ // Ascending order for key iteration
	})

	// Anchors will be set by main iterator logic

	return keys
}

func (ts *ThreadSorter) extractTimestampFromKey(key string, sortBy string) int64 {
	parsed, err := keys.ParseUserOwnsThread(key)
	if err != nil {
		return 0
	}

	threadParsed, err := keys.ParseKey(parsed.ThreadKey)
	if err != nil {
		return 0
	}

	switch sortBy {
	case "created_at", "created_ts":
		if ts, err := keys.KeyTimestampNumbered(threadParsed.ThreadTS); err == nil {
			return ts
		}
	case "updated_at", "updated_ts":
		if ts, err := keys.KeyTimestampNumbered(threadParsed.ThreadTS); err == nil {
			return ts
		}
	default:
		if ts, err := keys.KeyTimestampNumbered(threadParsed.ThreadTS); err == nil {
			return ts
		}
	}

	return 0
}

func (ts *ThreadSorter) sortByCreatedTS(threads []models.Thread) {
	sort.Slice(threads, func(i, j int) bool {
		tsI := threads[i].CreatedTS
		tsJ := threads[j].CreatedTS
		return tsI < tsJ // Ascending order
	})
}

func (ts *ThreadSorter) sortByUpdatedTS(threads []models.Thread) {
	sort.Slice(threads, func(i, j int) bool {
		tsI := threads[i].UpdatedTS
		tsJ := threads[j].UpdatedTS
		return tsI < tsJ // Ascending order
	})
}
