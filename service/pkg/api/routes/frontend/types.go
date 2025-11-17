package frontend

import (
	"progressdb/pkg/models"
	"progressdb/pkg/store/pagination"
)

type QueueExtras struct {
	ApiRole string `json:"api_role,omitempty"`
	UserID  string `json:"user_id"`
	ReqID   string `json:"reqid"`
	Remote  string `json:"remote,omitempty"`
}

type EnqueueRequest struct {
	Thread  string
	ID      string
	Payload []byte
	TS      int64
	Extras  QueueExtras
}

type ThreadsListResponse struct {
	Threads    []models.Thread                `json:"threads"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type ThreadResponse struct {
	Thread models.Thread `json:"thread"`
}

type MessagesListResponse struct {
	Thread     string                         `json:"thread"`
	Messages   []models.Message               `json:"messages"`
	Pagination *pagination.PaginationResponse `json:"pagination"`
}

type MessageResponse struct {
	Message models.Message `json:"message"`
}
