package tests

import (
	"fmt"
	"strings"
)

// SplitPath splits a path string into its non-empty segments, separated by '/'.
// For example, "/foo/bar/" becomes []string{"foo", "bar"}.
func SplitPath(p string) []string {
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

// MakeSlug creates a URL-friendly slug from a title and an ID.
// It lowercases the title, replaces non-alphanumeric characters with dashes, and appends the ID.
// If the resulting slug is empty, it defaults to "t-<id>".
func MakeSlug(title, id string) string {
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
