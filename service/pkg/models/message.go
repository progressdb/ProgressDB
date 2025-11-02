package models

type Message struct {
	Key    string `json:"key"`
	Thread string `json:"thread"`
	Author string `json:"author,omitempty"`
	Role   string `json:"role,omitempty"`

	CreatedTS int64 `json:"created_ts,omitempty"`
	UpdatedTS int64 `json:"updated_ts,omitempty"`

	Body    interface{} `json:"body,omitempty"`
	ReplyTo string      `json:"reply_to,omitempty"`
	Deleted bool        `json:"deleted,omitempty"`
}
