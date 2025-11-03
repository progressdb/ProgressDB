package pagination

import (
	"strconv"

	"github.com/valyala/fasthttp"
)

// ParseLegacyPaginationRequest parses the old cursor-based pagination (for backward compatibility)
func ParseLegacyPaginationRequest(ctx *fasthttp.RequestCtx) *PaginationRequest {
	req := &PaginationRequest{
		Limit: AdminDefaultLimit,
	}

	if limStr := string(ctx.QueryArgs().Peek("limit")); limStr != "" {
		if parsedLimit, err := strconv.Atoi(limStr); err == nil && parsedLimit > 0 && parsedLimit <= AdminMaxLimit {
			req.Limit = parsedLimit
		}
	}

	return req
}

// NewLegacyPaginationResponse creates the old-style response format (for backward compatibility)
func NewLegacyPaginationResponse(limit int, hasMore bool, cursor string, count int, total int) *PaginationResponse {
	return &PaginationResponse{
		Count: count,
		Total: total,
	}
}
