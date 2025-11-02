package pagination

type PaginationRequest struct {
	Before  string `json:"before,omitempty"`   // Fetch items older than this reference ID
	After   string `json:"after,omitempty"`    // Fetch items newer than this reference ID
	Anchor  string `json:"anchor,omitempty"`   // Fetch items around this anchor (takes precedence if set)
	Limit   int    `json:"limit,omitempty"`    // Max number to return
	SortBy  string `json:"sort_by,omitempty"`  // Sort by field: "created_at" or "updated_at"
	OrderBy string `json:"order_by,omitempty"` // "asc" for ascending, "desc" for descending
}

type PaginationResponse struct {
	StartAnchor string `json:"start_anchor"` // First item in the current page
	EndAnchor   string `json:"end_anchor"`   // Last item in the current page
	HasBefore   bool   `json:"has_before"`   // True if there are items before StartAnchor (previous page exists)
	HasAfter    bool   `json:"has_after"`    // True if there are items after EndAnchor (next page exists)
	OrderBy     string `json:"order_by"`     // Current sort order: "asc" or "desc"
	Count       int    `json:"count"`        // Number of items returned in this page
	Total       int    `json:"total"`        // Total number of items available
}
