package handlers

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

var idSeq uint64

// genID generates a unique message ID using the current UTC nanosecond timestamp and an atomic sequence number.
// The format is "msg-<timestamp>-<seq>".
func genID() string {
	n := time.Now().UTC().UnixNano()
	s := atomic.AddUint64(&idSeq, 1)
	return fmt.Sprintf("msg-%d-%d", n, s)
}

// genThreadID generates a unique thread ID using the current UTC nanosecond timestamp and an atomic sequence number.
// The format is "thread-<timestamp>-<seq>".
func genThreadID() string {
	n := time.Now().UTC().UnixNano()
	s := atomic.AddUint64(&idSeq, 1)
	return fmt.Sprintf("thread-%d-%d", n, s)
}

// splitPath splits a path string into its non-empty segments, separated by '/'.
// For example, "/foo/bar/" becomes []string{"foo", "bar"}.
func splitPath(p string) []string {
	out := make([]string, 0)
	cur := ""
	for i := 0; i < len(p); i++ {
		c := p[i]
		if c == '/' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// toRawMessages converts a slice of JSON-encoded strings to a slice of json.RawMessage.
func toRawMessages(vals []string) []json.RawMessage {
	out := make([]json.RawMessage, 0, len(vals))
	for _, s := range vals {
		out = append(out, json.RawMessage(s))
	}
	return out
}

// makeSlug creates a URL-friendly slug from a title and an ID.
// It lowercases the title, replaces non-alphanumeric characters with dashes, and appends the ID.
// If the resulting slug is empty, it defaults to "t-<id>".
func makeSlug(title, id string) string {
	t := strings.ToLower(title)
	var b strings.Builder
	lastDash := false
	for _, r := range t {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else {
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	s := strings.Trim(b.String(), "-")
	if s == "" {
		s = "t"
	}
	return fmt.Sprintf("%s-%s", s, id)
}
