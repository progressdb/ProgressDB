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

func (ts *ThreadSorter) SortThreads(threads []models.Thread, sortBy, orderBy string) []models.Thread {
	if len(threads) == 0 {
		return threads
	}

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
		ts.sortByCreatedTS(threads, orderBy)
	}

	return threads
}

func (ts *ThreadSorter) SortKeys(keys []string, sortBy, orderBy string, response *pagination.PaginationResponse) []string {
	if len(keys) == 0 {
		return keys
	}

	if sortBy == "" {
		sortBy = "created_at"
	}
	if orderBy == "" {
		orderBy = "desc"
	}

	sort.Slice(keys, func(i, j int) bool {
		tsI := ts.extractTimestampFromKey(keys[i], sortBy)
		tsJ := ts.extractTimestampFromKey(keys[j], sortBy)

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
		if ts, err := keys.ParseKeyTimestamp(threadParsed.ThreadTS); err == nil {
			return ts
		}
	case "updated_at", "updated_ts":
		if ts, err := keys.ParseKeyTimestamp(threadParsed.ThreadTS); err == nil {
			return ts
		}
	default:
		if ts, err := keys.ParseKeyTimestamp(threadParsed.ThreadTS); err == nil {
			return ts
		}
	}

	return 0
}

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
