package pagination

import (
	"strconv"
	"strings"

	"github.com/valyala/fasthttp"
)

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

func NewPaginationResponse(limit int, hasMore bool, cursor string, count int, total int) *PaginationResponse {
	return &PaginationResponse{
		Limit:   limit,
		HasMore: hasMore,
		Cursor:  cursor,
		Count:   count,
		Total:   total,
	}
}
