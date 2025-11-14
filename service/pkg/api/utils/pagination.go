package utils

import (
	"fmt"

	"progressdb/pkg/store/pagination"

	"github.com/valyala/fasthttp"
)

func ParsePaginationRequest(ctx *fasthttp.RequestCtx) pagination.PaginationRequest {
	req := pagination.PaginationRequest{
		Before: GetQuery(ctx, "before"),
		After:  GetQuery(ctx, "after"),
		Anchor: GetQuery(ctx, "anchor"),
		SortBy: GetQuery(ctx, "sort_by"),
	}

	// Parse limit - keep 0 if explicitly provided to catch invalid limits
	req.Limit = GetQueryInt(ctx, "limit", 0)

	// Set default sort_by only
	if req.SortBy == "" {
		req.SortBy = "created_ts" // Default to created_ts
	}

	return req
}

func ValidatePaginationRequest(req *pagination.PaginationRequest, ctx *fasthttp.RequestCtx) error {
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
	if req.SortBy != "" && req.SortBy != "created_ts" && req.SortBy != "updated_ts" {
		return fmt.Errorf("sort_by must be 'created_ts' or 'updated_ts'")
	}

	// Check if limit was explicitly provided
	userLimitProvided := ctx.QueryArgs().Has("limit")

	// Validate limit using constants
	if req.Limit < 0 {
		return fmt.Errorf("limit cannot be negative")
	}
	if userLimitProvided && req.Limit == 0 {
		return fmt.Errorf("limit must be at least 1")
	}
	if req.Limit > 0 && req.Limit < 1 {
		return fmt.Errorf("limit must be at least 1")
	}
	if req.Limit > pagination.MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", pagination.MaxLimit)
	}

	// Apply default limit if not set (after validation)
	if req.Limit == 0 {
		req.Limit = pagination.DefaultLimit
	}

	return nil
}
