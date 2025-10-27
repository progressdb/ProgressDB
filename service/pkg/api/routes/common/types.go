package common

// RequestMetadata represents common metadata extracted from HTTP requests
type RequestMetadata struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	ReqID  string `json:"reqid"`
	Remote string `json:"remote"`
}

type QueueExtras struct {
	Role   string `json:"role,omitempty"`
	UserID string `json:"user_id"` // not omitempty, must not be empty
	ReqID  string `json:"reqid"`   // not omitempty, must not be empty
	Remote string `json:"remote,omitempty"`
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
