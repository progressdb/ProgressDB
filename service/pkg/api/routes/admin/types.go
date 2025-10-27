package admin

import (
	"encoding/json"
	"progressdb/pkg/api/routes/common"
)

type DashboardKeysResult struct {
	Keys       []string                   `json:"keys"`
	Pagination *common.PaginationResponse `json:"pagination"`
}

type DashboardUsersResult struct {
	Users      []string                   `json:"users"`
	Pagination *common.PaginationResponse `json:"pagination"`
}

type DashboardThreadsResult struct {
	Threads    []json.RawMessage          `json:"threads"`
	Pagination *common.PaginationResponse `json:"pagination"`
}

type DashboardMessagesResult struct {
	Messages   []json.RawMessage          `json:"messages"`
	Pagination *common.PaginationResponse `json:"pagination"`
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
