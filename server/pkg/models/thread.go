package models

type Thread struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	// Author is an opaque identity id (clients manage meaning); default empty string
	Author string `json:"author"`
	// Slug is generated from title and id for human-friendly URLs
	Slug string `json:"slug,omitempty"`
	// Created timestamp (ns)
	CreatedTS int64 `json:"created_ts,omitempty"`
	// Updated timestamp (ns) - last time metadata or thread activity changed
	UpdatedTS int64 `json:"updated_ts,omitempty"`
}
