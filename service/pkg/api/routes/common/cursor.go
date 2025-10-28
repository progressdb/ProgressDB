package common

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
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
	ThreadID  string `json:"thread_id"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}

func ParsePaginationRequest(ctx *fasthttp.RequestCtx) *PaginationRequest {
	req := &PaginationRequest{
		Limit:  100,
		Cursor: strings.TrimSpace(string(ctx.QueryArgs().Peek("cursor"))),
	}

	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if parsedLimit, err := strconv.Atoi(limStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			req.Limit = parsedLimit
		}
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

func EncodeMessageCursor(threadID string, timestamp int64, sequence uint64) (string, error) {
	cursor := MessageCursor{
		ThreadID:  threadID,
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
