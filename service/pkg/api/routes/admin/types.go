package admin

import (
	"progressdb/pkg/store/pagination"
)

type DashboardKeysResult struct {
	Keys       []string                      `json:"keys"`
	Pagination pagination.PaginationResponse `json:"pagination"`
}

type DashboardUsersResult struct {
	Users      []string                      `json:"users"`
	Pagination pagination.PaginationResponse `json:"pagination"`
}

type DashboardThreadsResult struct {
	Threads    []string                      `json:"threads"`
	Pagination pagination.PaginationResponse `json:"pagination"`
}

type DashboardMessagesResult struct {
	Messages   []string                      `json:"messages"`
	Pagination pagination.PaginationResponse `json:"pagination"`
}

type DashboardEncryptJobResult struct {
	Key     string `json:"key"`
	DEKKey  string `json:"dek_key"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type EncryptionEncryptRequest struct {
	Keys        []string `json:"keys"`
	All         bool     `json:"all"`
	Parallelism int      `json:"parallelism"`
}
