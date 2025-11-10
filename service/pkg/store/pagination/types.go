package pagination

type PaginationRequest struct {
	Before string `json:"before,omitempty"`  // Fetch items older than this reference ID
	After  string `json:"after,omitempty"`   // Fetch items newer than this reference ID
	Anchor string `json:"anchor,omitempty"`  // Fetch items around this anchor (takes precedence if set)
	Limit  int    `json:"limit,omitempty"`   // Max number to return
	SortBy string `json:"sort_by,omitempty"` // Sort by field: "created_ts" or "updated_ts"
}

type PaginationResponse struct {
	BeforeAnchor string `json:"before_anchor"` // Use this to get previous page
	AfterAnchor  string `json:"after_anchor"`  // Use this to get next page
	HasBefore    bool   `json:"has_before"`    // True if there are items before BeforeAnchor (previous page exists)
	HasAfter     bool   `json:"has_after"`     // True if there are items after AfterAnchor (next page exists)
	Count        int    `json:"count"`         // Number of items returned in this page
	Total        int    `json:"total"`         // Total number of items available
}
