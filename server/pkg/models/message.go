package models

type Message struct {
    ID     string      `json:"id"`
    Thread string      `json:"thread"`
    Author string      `json:"author,omitempty"`
    TS     int64       `json:"ts"`
    Body   interface{} `json:"body,omitempty"`
}

