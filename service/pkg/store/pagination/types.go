package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type CursorPayload struct {
	LastThreadKey  string `json:"last_thread_key,omitempty"`  // key of the last thread fetched
	LastMessageKey string `json:"last_message_key,omitempty"` // key of the last message fetched
}

type PaginationRequest struct {
	Limit  int    `json:"limit,omitempty"`  // number of items to fetch per page
	Cursor string `json:"cursor,omitempty"` // key of the last item fetched
}

type PaginationResponse struct {
	Limit      int    `json:"limit"`                 // number of items to fetch per page
	HasMore    bool   `json:"has_more"`              // true if there are more items to fetch
	NextCursor string `json:"next_cursor,omitempty"` // key of the next item to fetch
	Count      int    `json:"count"`                 // number of items returned
	Total      int    `json:"total,omitempty"`       // complete total number of items
}

func EncodeCursor(payload CursorPayload) string {
	b, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}

func DecodeCursor(cursor string) (CursorPayload, error) {
	var cp CursorPayload
	if cursor == "" {
		return cp, nil
	}
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return cp, fmt.Errorf("decode base64: %w", err)
	}
	if err := json.Unmarshal(data, &cp); err != nil {
		return cp, fmt.Errorf("decode cursor JSON: %w", err)
	}
	return cp, nil
}
