package tests

import (
	"testing"

	"progressdb/pkg/api/utils"
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

			// Parse pagination
			req := utils.ParsePaginationRequest(ctx)

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
			err := utils.ValidatePaginationRequest(tt.req)

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
