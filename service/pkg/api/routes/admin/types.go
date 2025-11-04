package admin

import (
	"progressdb/pkg/store/pagination"
)

// Dashboard pagination results
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
