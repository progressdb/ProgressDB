package common

import (
	"encoding/base64"
	"encoding/json"

	"github.com/valyala/fasthttp"
	"progressdb/pkg/api/utils"
)

type PaginationRequest struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type PaginationResponse struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Count      int    `json:"count"`
	Total      int    `json:"total,omitempty"`
}

type MessageCursor struct {
	ThreadKey string `json:"thread_key"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}

func ParsePaginationRequest(ctx *fasthttp.RequestCtx) *PaginationRequest {
	req := &PaginationRequest{
		Limit:  utils.GetQueryInt(ctx, "limit", 100),
		Cursor: utils.GetQuery(ctx, "cursor"),
	}

	// Enforce maximum limit
	if req.Limit > 1000 {
		req.Limit = 1000
	}

	return req
}

func NewPaginationResponse(limit int, hasMore bool, nextCursor string, count int, total int) *PaginationResponse {
	return &PaginationResponse{
		Limit:      limit,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Count:      count,
		Total:      total,
	}
}

func EncodeMessageCursor(threadKey string, timestamp int64, sequence uint64) (string, error) {
	cursor := MessageCursor{
		ThreadKey: threadKey,
		Timestamp: timestamp,
		Sequence:  sequence,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func DecodeMessageCursor(cursor string) (*MessageCursor, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	var mc MessageCursor
	err = json.Unmarshal(data, &mc)
	if err != nil {
		return nil, err
	}
	return &mc, nil
}
