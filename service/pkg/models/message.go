package models

type Message struct {
	Key     string      `json:"key"`
	Thread  string      `json:"thread"`
	Author  string      `json:"author,omitempty"`
	Role    string      `json:"role,omitempty"`
	TS      int64       `json:"ts"`
	Body    interface{} `json:"body,omitempty"`
	ReplyTo string      `json:"reply_to,omitempty"`
	Deleted bool        `json:"deleted,omitempty"`
}
