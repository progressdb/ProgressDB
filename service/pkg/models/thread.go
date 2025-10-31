package models

type Thread struct {
	Key   string `json:"key"`
	Title string `json:"title,omitempty"`
	// Author is an opaque identity id (clients manage meaning); default empty string
	Author string `json:"author"`
	// Slug is generated from title and key for human-friendly URLs
	Slug string `json:"slug,omitempty"`
	// Created timestamp (ns)
	CreatedTS int64 `json:"created_ts,omitempty"`
	// Updated timestamp (ns) - last time metadata or thread activity changed
	UpdatedTS int64 `json:"updated_ts,omitempty"`
	// LastSeq is a per-thread sequence number, incremented and persisted with each message.
	LastSeq uint64 `json:"last_seq,omitempty"`
	// Deleted marks a thread as soft-deleted; DeletedTS records deletion time (ns)
	Deleted bool `json:"deleted,omitempty"`

	// KMS holds optional per-thread DEK metadata used for encrypting child messages.
	// Use a pointer so the field can be omitted entirely when not set.
	KMS *KMSMeta `json:"kms,omitempty"`
}

type KMSMeta struct {
	KeyID      string `json:"key_id,omitempty"`
	WrappedDEK string `json:"wrapped_dek,omitempty"`
	KEKID      string `json:"kek_id,omitempty"`
	KEKVersion string `json:"kek_version,omitempty"`
}

// Extend Thread with optional KMS metadata
func (t *Thread) WithKMS(meta KMSMeta) {
	t.KMS = &meta
}
