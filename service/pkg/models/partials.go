package models

type ThreadUpdatePartial struct {
	Key       string `json:"key"`
	UpdatedTS int64  `json:"updated_ts"`
	Title     string `json:"title"`
}

type MessageUpdatePartial struct {
	Key       string      `json:"key"`
	Thread    string      `json:"thread"`
	Body      interface{} `json:"body"`
	UpdatedTS int64       `json:"updated_ts"`
}

type ThreadDeletePartial struct {
	Key       string `json:"key"`
	UpdatedTS int64  `json:"updated_ts"`
}

type MessageDeletePartial struct {
	Key       string `json:"key"`
	Deleted   bool   `json:"deleted"`
	UpdatedTS int64  `json:"updated_ts"`
	Thread    string `json:"thread"`
	Author    string `json:"author"`
}
