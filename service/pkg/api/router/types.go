package router

// RequestMetadata represents common metadata extracted from HTTP requests
type RequestMetadata struct {
	ApiRole string `json:"api_role"`
	UserID  string `json:"user_id"`
	ReqID   string `json:"reqid"`
	ReqIP   string `json:"req_ip"`
}

type QueueExtras struct {
	ApiRole string `json:"api_role,omitempty"`
	UserID  string `json:"user_id"` // not omitempty, must not be empty
	ReqID   string `json:"reqid"`   // not omitempty, must not be empty
	ReqIP   string `json:"req_ip,omitempty"`
}

type EnqueueRequest struct {
	Thread  string
	ID      string
	Payload []byte
	TS      int64
	Extras  QueueExtras
}

type QueryParameters struct {
	Limit  int
	Cursor string
}
