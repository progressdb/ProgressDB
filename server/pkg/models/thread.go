package models

type Thread struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	// Created timestamp (ns)
	CreatedTS int64 `json:"created_ts,omitempty"`
}
