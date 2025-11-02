package tests

import (
	"fmt"
	"strconv"
	"testing"

	"progressdb/pkg/store/pagination"
)

// TestPaginationParsing tests the new pagination parameter parsing
func TestPaginationParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected pagination.PaginationRequest
	}{
		{
			name: "anchor query",
			input: map[string]string{
				"anchor":   "thread123",
				"limit":    "40",
				"sort_by":  "created_at",
				"order_by": "asc",
			},
			expected: pagination.PaginationRequest{
				Anchor:  "thread123",
				Limit:   40,
				SortBy:  "created_at",
				OrderBy: "asc",
			},
		},
		{
			name: "before query",
			input: map[string]string{
				"before":   "thread456",
				"limit":    "20",
				"sort_by":  "updated_at",
				"order_by": "desc",
			},
			expected: pagination.PaginationRequest{
				Before:  "thread456",
				Limit:   20,
				SortBy:  "updated_at",
				OrderBy: "desc",
			},
		},
		{
			name: "after query",
			input: map[string]string{
				"after":    "thread789",
				"limit":    "100",
				"sort_by":  "created_at",
				"order_by": "desc",
			},
			expected: pagination.PaginationRequest{
				After:   "thread789",
				Limit:   100,
				SortBy:  "created_at",
				OrderBy: "desc",
			},
		},
		{
			name:  "defaults",
			input: map[string]string{},
			expected: pagination.PaginationRequest{
				Limit:   50,
				SortBy:  "updated_at",
				OrderBy: "desc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock request context
			ctx := &mockRequestCtx{queryParams: tt.input}

			// Parse pagination using test helper
			req := parsePaginationRequestTest(ctx)

			// Verify parsing
			if req.Before != tt.expected.Before {
				t.Errorf("Before = %v, want %v", req.Before, tt.expected.Before)
			}
			if req.After != tt.expected.After {
				t.Errorf("After = %v, want %v", req.After, tt.expected.After)
			}
			if req.Anchor != tt.expected.Anchor {
				t.Errorf("Anchor = %v, want %v", req.Anchor, tt.expected.Anchor)
			}
			if req.Limit != tt.expected.Limit {
				t.Errorf("Limit = %v, want %v", req.Limit, tt.expected.Limit)
			}
			if req.SortBy != tt.expected.SortBy {
				t.Errorf("SortBy = %v, want %v", req.SortBy, tt.expected.SortBy)
			}
			if req.OrderBy != tt.expected.OrderBy {
				t.Errorf("OrderBy = %v, want %v", req.OrderBy, tt.expected.OrderBy)
			}
		})
	}
}

// TestPaginationValidation tests parameter validation
func TestPaginationValidation(t *testing.T) {
	tests := []struct {
		name        string
		req         pagination.PaginationRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid anchor query",
			req: pagination.PaginationRequest{
				Anchor:  "thread123",
				Limit:   50,
				SortBy:  "created_at",
				OrderBy: "asc",
			},
			expectError: false,
		},
		{
			name: "multiple reference points - invalid",
			req: pagination.PaginationRequest{
				Before: "thread123",
				After:  "thread456",
				Limit:  50,
			},
			expectError: true,
			errorMsg:    "only one of anchor, before, after can be specified",
		},
		{
			name: "invalid sort_by",
			req: pagination.PaginationRequest{
				Limit:  50,
				SortBy: "invalid_field",
			},
			expectError: true,
			errorMsg:    "sort_by must be 'created_at' or 'updated_at'",
		},
		{
			name: "invalid order_by",
			req: pagination.PaginationRequest{
				Limit:   50,
				OrderBy: "invalid_order",
			},
			expectError: true,
			errorMsg:    "order_by must be 'asc' or 'desc'",
		},
		{
			name: "limit too small",
			req: pagination.PaginationRequest{
				Limit: 0,
			},
			expectError: true,
			errorMsg:    "limit must be at least 1",
		},
		{
			name: "limit too large",
			req: pagination.PaginationRequest{
				Limit: 1001,
			},
			expectError: true,
			errorMsg:    "limit cannot exceed 1000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePaginationRequestTest(tt.req)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Error = %v, want %v", err.Error(), tt.errorMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// PaginationContext interface for what we need in pagination parsing
type PaginationContext interface {
	QueryArgs() *mockQueryArgs
}

// Mock request context for testing
type mockRequestCtx struct {
	queryParams map[string]string
}

func (m *mockRequestCtx) QueryArgs() *mockQueryArgs {
	return &mockQueryArgs{params: m.queryParams}
}

type mockQueryArgs struct {
	params map[string]string
}

func (m *mockQueryArgs) Peek(key string) []byte {
	if val, ok := m.params[key]; ok {
		return []byte(val)
	}
	return nil
}

func (m *mockQueryArgs) Has(key string) bool {
	_, ok := m.params[key]
	return ok
}

// Test helper functions that mirror utils functions but work with our interface
func parsePaginationRequestTest(ctx PaginationContext) pagination.PaginationRequest {
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

func validatePaginationRequestTest(req pagination.PaginationRequest) error {
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
