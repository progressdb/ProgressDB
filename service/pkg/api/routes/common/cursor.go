package common

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
)

// PaginationRequest represents standardized pagination parameters from requests
type PaginationRequest struct {
	Limit  int    `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// PaginationResponse represents standardized pagination metadata in responses
type PaginationResponse struct {
	Limit      int    `json:"limit"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Count      int    `json:"count"`
}

// MessageCursor represents cursor data for message pagination
type MessageCursor struct {
	ThreadID  string `json:"thread_id"`
	Timestamp int64  `json:"timestamp"`
	Sequence  uint64 `json:"sequence"`
}

// ThreadCursor represents cursor data for thread pagination
type ThreadCursor struct {
	UserID    string `json:"user_id"`
	Timestamp int64  `json:"timestamp"`
	ThreadID  string `json:"thread_id"`
}

// ParsePaginationRequest extracts standardized pagination parameters from request
func ParsePaginationRequest(ctx *fasthttp.RequestCtx) *PaginationRequest {
	req := &PaginationRequest{
		Limit:  100, // Default limit
		Cursor: strings.TrimSpace(string(ctx.QueryArgs().Peek("cursor"))),
	}

	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if parsedLimit, err := strconv.Atoi(limStr); err == nil && parsedLimit > 0 && parsedLimit <= 1000 {
			req.Limit = parsedLimit
		}
	}

	return req
}

// NewPaginationResponse creates a standardized pagination response
func NewPaginationResponse(limit int, hasMore bool, nextCursor string, count int) *PaginationResponse {
	return &PaginationResponse{
		Limit:      limit,
		HasMore:    hasMore,
		NextCursor: nextCursor,
		Count:      count,
	}
}

// EncodeMessageCursor encodes message cursor data to opaque token
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

// DecodeMessageCursor decodes message cursor token to data
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

// EncodeThreadCursor encodes thread cursor data to opaque token
func EncodeThreadCursor(userID, threadID string, timestamp int64) (string, error) {
	cursor := ThreadCursor{
		UserID:    userID,
		Timestamp: timestamp,
		ThreadID:  threadID,
	}
	data, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// DecodeThreadCursor decodes thread cursor token to data
func DecodeThreadCursor(cursor string) (*ThreadCursor, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return nil, err
	}
	var tc ThreadCursor
	err = json.Unmarshal(data, &tc)
	if err != nil {
		return nil, err
	}
	return &tc, nil
}
