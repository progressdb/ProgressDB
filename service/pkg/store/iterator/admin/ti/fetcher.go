package ti

import (
	"encoding/json"

	"progressdb/pkg/models"
	thread_store "progressdb/pkg/store/features/threads"
)

// ThreadFetcher handles fetching thread data from keys
type ThreadFetcher struct{}

// NewThreadFetcher creates a new thread fetcher
func NewThreadFetcher() *ThreadFetcher {
	return &ThreadFetcher{}
}

// FetchThreads fetches thread data for the given keys
func (tf *ThreadFetcher) FetchThreads(threadKeys []string, author string) ([]models.Thread, error) {
	threads := make([]models.Thread, 0, len(threadKeys))

	for _, threadKey := range threadKeys {
		threadData, err := thread_store.GetThreadData(threadKey)
		if err != nil {
			continue // Skip threads that can't be loaded
		}

		var thread models.Thread
		if err := json.Unmarshal([]byte(threadData), &thread); err != nil {
			continue // Skip invalid thread data
		}

		// Only include threads that belong to the author
		if thread.Author == author {
			threads = append(threads, thread)
		}
	}

	return threads, nil
}
