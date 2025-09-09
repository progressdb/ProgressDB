package models

type Message struct {
    ID     string      `json:"id"`
    Thread string      `json:"thread"`
    Author string      `json:"author,omitempty"`
    // Role represents the actor role for this message (e.g. "user", "system").
    // Defaults to "user" when omitted.
    Role   string      `json:"role,omitempty"`
    TS     int64       `json:"ts"`
    Body   interface{} `json:"body,omitempty"`
	// Optional reply-to message ID
	ReplyTo string `json:"reply_to,omitempty"`
	// Deleted flag; soft-delete implemented as an appended tombstone version
	Deleted bool `json:"deleted,omitempty"`
	// Reactions is an optional map of identity id -> reaction string.
	// The identity id is an opaque identifier whose meaning is known to
	// the client (it may represent a user, a group, or any identity).
	Reactions map[string]string `json:"reactions,omitempty"`
}
