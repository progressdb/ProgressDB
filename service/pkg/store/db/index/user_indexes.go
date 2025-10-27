package index

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
	"progressdb/pkg/telemetry"
)

func GetUserThreads(userID string) ([]string, error) {
	prefix := keys.GenUserThreadRelPrefix(userID)
	keys, _, err := ListKeysWithPrefixPaginated(prefix, &pagination.PaginationRequest{Limit: 10000, Cursor: ""})
	if err != nil {
		return nil, fmt.Errorf("list user ownership keys: %w", err)
	}
	threads := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) >= 5 && parts[0] == "rel" && parts[1] == "u" && parts[2] == userID && parts[3] == "t" {
			threads = append(threads, parts[4])
		}
	}
	return threads, nil
}

type ThreadWithTimestamp struct {
	Key       string
	Timestamp int64
}

func GetUserThreadsCursor(userID, cursor string, limit int) ([]string, *pagination.PaginationResponse, error) {
	tr := telemetry.Track("index.get_user_threads_cursor")
	defer tr.Finish()

	threadKeys, err := GetUserThreads(userID)
	if err != nil {
		return nil, nil, err
	}

	threads := make([]ThreadWithTimestamp, 0, len(threadKeys))
	for _, threadKey := range threadKeys {
		threadKey := keys.GenThreadKey(threadKey)
		threadData, err := GetKey(threadKey)
		if err != nil {
			continue
		}

		var threadMeta struct {
			CreatedAt int64 `json:"created_at"`
		}
		if err := json.Unmarshal([]byte(threadData), &threadMeta); err != nil {
			continue
		}

		threads = append(threads, ThreadWithTimestamp{
			Key:       threadKey,
			Timestamp: threadMeta.CreatedAt,
		})
	}

	sort.Slice(threads, func(i, j int) bool {
		return threads[i].Timestamp > threads[j].Timestamp
	})

	startIndex := 0
	if cursor != "" {
		lastKey, err := pagination.DecodeCursor(cursor)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid cursor: %w", err)
		}
		for i, t := range threads {
			if t.Key == lastKey {
				startIndex = i + 1
				break
			}
		}
	}

	endIndex := startIndex + limit
	if endIndex > len(threads) {
		endIndex = len(threads)
	}

	if startIndex >= len(threads) {
		return []string{}, pagination.NewPaginationResponse(limit, false, "", 0), nil
	}

	pageThreads := threads[startIndex:endIndex]
	threadKeysOnly := make([]string, len(pageThreads))
	for i, t := range pageThreads {
		threadKeysOnly[i] = t.Key
	}

	hasMore := endIndex < len(threads)

	var nextCursor string
	if hasMore && len(pageThreads) > 0 {
		lastThread := pageThreads[len(pageThreads)-1]
		nextCursor = lastThread.Key
	}

	return threadKeysOnly, pagination.NewPaginationResponse(limit, hasMore, pagination.EncodeCursor(nextCursor), len(threadKeysOnly)), nil
}

func GetThreadParticipants(threadKey string) ([]string, error) {
	prefix := keys.GenThreadUserRelPrefix(threadKey)
	keys, _, err := ListKeysWithPrefixPaginated(prefix, &pagination.PaginationRequest{Limit: 10000, Cursor: ""})
	if err != nil {
		return nil, fmt.Errorf("list thread participant keys: %w", err)
	}
	users := make([]string, 0, len(keys))
	for _, key := range keys {
		parts := strings.Split(key, ":")
		if len(parts) >= 5 && parts[0] == "rel" && parts[1] == "t" && parts[2] == threadKey && parts[3] == "u" {
			users = append(users, parts[4])
		}
	}
	return users, nil
}
