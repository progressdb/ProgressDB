package models

type Message struct {
    ID     string      `json:"id"`
    Thread string      `json:"thread"`
    Author string      `json:"author,omitempty"`
    TS     int64       `json:"ts"`
    Body   interface{} `json:"body,omitempty"`
    // Optional reply-to message ID
    ReplyTo string `json:"reply_to,omitempty"`
    // Deleted flag; soft-delete implemented as an appended tombstone version
    Deleted bool `json:"deleted,omitempty"`
    // Reactions is a map of reaction key -> count
    Reactions map[string]int `json:"reactions,omitempty"`
}
