package admin

import (
	"progressdb/pkg/store/pagination"
)

type DashboardKeysResult struct {
	Keys       []string                       `json:"keys"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type DashboardUsersResult struct {
	Users      []string                       `json:"users"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type DashboardThreadsResult struct {
	Threads    []string                       `json:"threads"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type DashboardMessagesResult struct {
	Messages   []string                       `json:"messages"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type DashboardRewrapJobResult struct {
	Key     string `json:"key"`
	OldKEK  string `json:"old_kek,omitempty"`
	NewKEK  string `json:"new_kek,omitempty"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type DashboardEncryptJobResult struct {
	Key     string `json:"key"`
	DEKKey  string `json:"dek_key"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

type EncryptionRotateRequest struct {
	Key string `json:"key"`
}

type EncryptionRewrapRequest struct {
	Keys        []string `json:"keys"`
	All         bool     `json:"all"`
	NewKEKHex   string   `json:"new_kek_hex"`
	Parallelism int      `json:"parallelism"`
}

type EncryptionEncryptRequest struct {
	Keys        []string `json:"keys"`
	All         bool     `json:"all"`
	Parallelism int      `json:"parallelism"`
}
