package admin

import "encoding/json"

type AdminKeysResult struct {
	Keys       []string `json:"keys"`
	NextCursor string   `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
	Count      int      `json:"count"`
}

type AdminUsersResult struct {
	Users      []string `json:"users"`
	NextCursor string   `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
	Count      int      `json:"count"`
}

type AdminThreadsResult struct {
	Threads    []json.RawMessage `json:"threads"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
	Count      int               `json:"count"`
}

type AdminMessagesResult struct {
	Messages   []json.RawMessage `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
	Count      int               `json:"count"`
}

type RewrapResult struct {
	Key string
	Err string
	Kek string
}

type EncryptResult struct {
	Thread string
	Key    string
	Err    string
}
