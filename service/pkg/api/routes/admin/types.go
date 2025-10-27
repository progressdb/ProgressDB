package admin

import "encoding/json"

type DashboardKeysResult struct {
	Keys       []string `json:"keys"`
	NextCursor string   `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
	Count      int      `json:"count"`
}

type DashboardUsersResult struct {
	Users      []string `json:"users"`
	NextCursor string   `json:"next_cursor,omitempty"`
	HasMore    bool     `json:"has_more"`
	Count      int      `json:"count"`
}

type DashboardThreadsResult struct {
	Threads    []json.RawMessage `json:"threads"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
	Count      int               `json:"count"`
}

type DashboardMessagesResult struct {
	Messages   []json.RawMessage `json:"messages"`
	NextCursor string            `json:"next_cursor,omitempty"`
	HasMore    bool              `json:"has_more"`
	Count      int               `json:"count"`
}

type DashboardRewrapJobResult struct {
	Key     string `json:"key"`
	OldKEK  string `json:"old_kek,omitempty"`
	NewKEK  string `json:"new_kek,omitempty"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type DashboardEncryptJobResult struct {
	Thread  string `json:"thread"`
	Key     string `json:"key"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}
