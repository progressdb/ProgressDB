package utils

import (
	"fmt"
	"strconv"

	"github.com/valyala/fasthttp"
	"progressdb/pkg/store/pagination"
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

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 50
	}
	if req.OrderBy == "" {
		req.OrderBy = "desc" // Default to descending for threads
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

	// Validate limit
	if req.Limit < 1 {
		return fmt.Errorf("limit must be at least 1")
	}
	if req.Limit > 1000 {
		return fmt.Errorf("limit cannot exceed 1000")
	}

	return nil
}
