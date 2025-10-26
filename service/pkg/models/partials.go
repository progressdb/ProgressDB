package models

type ThreadUpdatePartial struct {
	Key       string `json:"key"`
	UpdatedTS int64  `json:"updated_ts"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
}

type MessageUpdatePartial struct {
	Key    string      `json:"key"`
	Thread string      `json:"thread"`
	Body   interface{} `json:"body"`
	TS     int64       `json:"ts"`
}

type ThreadDeletePartial struct {
	Key string `json:"key"`
}

type MessageDeletePartial struct {
	Key     string `json:"key"`
	Deleted bool   `json:"deleted"`
	TS      int64  `json:"ts"`
	Thread  string `json:"thread"`
	Author  string `json:"author"`
}
