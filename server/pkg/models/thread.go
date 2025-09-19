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
	// Deleted marks a thread as soft-deleted; DeletedTS records deletion time (ns)
	Deleted   bool  `json:"deleted,omitempty"`
	DeletedTS int64 `json:"deleted_ts,omitempty"`

	// KMS holds optional per-thread DEK metadata used for encrypting child messages.
	KMS KMSMeta `json:"kms,omitempty"`
}

type KMSMeta struct {
	KeyID      string `json:"key_id,omitempty"`
	WrappedDEK string `json:"wrapped_dek,omitempty"`
	KEKID      string `json:"kek_id,omitempty"`
	KEKVersion string `json:"kek_version,omitempty"`
}

// Extend Thread with optional KMS metadata
func (t *Thread) WithKMS(meta KMSMeta) {
	// note: we add the field dynamically via JSON when marshaling; keep simple accessor
}
