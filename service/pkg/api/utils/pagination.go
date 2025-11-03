package utils

import (
	"fmt"
	"strconv"

	"progressdb/pkg/store/pagination"

	"github.com/valyala/fasthttp"
)

// ParsePaginationRequest parses new pagination parameters from HTTP request
func ParsePaginationRequest(ctx *fasthttp.RequestCtx) pagination.PaginationRequest {
	req := pagination.PaginationRequest{
		Before:  string(ctx.QueryArgs().Peek("before")),
		After:   string(ctx.QueryArgs().Peek("after")),
		Anchor:  string(ctx.QueryArgs().Peek("anchor")),
		SortBy:  string(ctx.QueryArgs().Peek("sort_by")),
		OrderBy: string(ctx.QueryArgs().Peek("order_by")),
	}

	// Parse limit with default
	if limitStr := string(ctx.QueryArgs().Peek("limit")); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}

	// Set defaults using constants
	if req.Limit == 0 {
		req.Limit = pagination.DefaultLimit
	}
	if req.OrderBy == "" {
		req.OrderBy = "asc" // Default to ascending for chronological order
	}
	if req.SortBy == "" {
		req.SortBy = "updated_at" // Default to updated_at for threads
	}

	return req
}

// ValidatePaginationRequest validates pagination parameters
func ValidatePaginationRequest(req pagination.PaginationRequest) error {
	// Only one of anchor, before, after can be set
	refCount := 0
	if req.Anchor != "" {
		refCount++
	}
	if req.Before != "" {
		refCount++
	}
	if req.After != "" {
		refCount++
	}

	if refCount > 1 {
		return fmt.Errorf("only one of anchor, before, after can be specified")
	}

	// Validate sort_by
	if req.SortBy != "" && req.SortBy != "created_at" && req.SortBy != "updated_at" {
		return fmt.Errorf("sort_by must be 'created_at' or 'updated_at'")
	}

	// Validate order_by
	if req.OrderBy != "" && req.OrderBy != "asc" && req.OrderBy != "desc" {
		return fmt.Errorf("order_by must be 'asc' or 'desc'")
	}

	// Validate limit using constants
	if req.Limit < 1 {
		return fmt.Errorf("limit must be at least 1")
	}
	if req.Limit > pagination.MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", pagination.MaxLimit)
	}

	return nil
}
