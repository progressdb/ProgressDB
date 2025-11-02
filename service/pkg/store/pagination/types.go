package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

type CursorPayload struct {
	LastListItemKey string `json:"last_list_item_key,omitempty"` // key of the last item fetched
	ListOrder       string `json:"list_order,omitempty"`         // oldest-first, latest-first
}

type PaginationRequest struct {
	Before  string `json:"before,omitempty"`   // Fetch items older than this reference ID
	After   string `json:"after,omitempty"`    // Fetch items newer than this reference ID
	Anchor  string `json:"anchor,omitempty"`   // Fetch items around this anchor (takes precedence if set)
	Limit   int    `json:"limit,omitempty"`    // Max number to return
	SortBy  string `json:"sort_by,omitempty"`  // Sort by field: "created_at" or "updated_at"
	OrderBy string `json:"order_by,omitempty"` // "asc" for ascending, "desc" for descending
}

type PaginationResponse struct {
	StartAnchor string `json:"start_anchor,omitempty"`
	EndAnchor   string `json:"end_anchor,omitempty"`
	HasBefore   bool   `json:"has_before"`         // NEW: More items before start_anchor
	HasAfter    bool   `json:"has_after"`          // NEW: More items after end_anchor
	OrderBy     string `json:"order_by,omitempty"` // "asc" for ascending, "desc" for descending
	Count       int    `json:"count"`              // number of items returned
	Total       int    `json:"total,omitempty"`    // complete total number of items (messages, threads, etc)
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
