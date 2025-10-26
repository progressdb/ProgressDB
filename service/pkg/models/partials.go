package models

type ThreadUpdatePartial struct {
	ID        string `json:"id"`
	UpdatedTS int64  `json:"updated_ts"`
	Title     string `json:"title"`
	Slug      string `json:"slug"`
}

type MessageUpdatePartial struct {
	ID     string      `json:"id"`
	Thread string      `json:"thread"`
	Body   interface{} `json:"body"`
	TS     int64       `json:"ts"`
}

type ThreadDeletePartial struct {
	ID string `json:"id"`
}

type DeletePartial struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
	TS      int64  `json:"ts"`
	Thread  string `json:"thread"`
	Author  string `json:"author"`
}
