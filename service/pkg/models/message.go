package models

type Message struct {
	Key    string `json:"key"`
	Thread string `json:"thread"`
	Author string `json:"author"`

	CreatedTS int64 `json:"created_ts,omitempty"`
	UpdatedTS int64 `json:"updated_ts,omitempty"`

	Body    interface{} `json:"body,omitempty"`
	Deleted bool        `json:"deleted,omitempty"`
}
