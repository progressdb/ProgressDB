package slug

import (
	"regexp"
	"strings"
	"unicode"

	"progressdb/pkg/store/keys"
)

// GenerateSlug creates a URL-friendly slug from a title and thread key
func GenerateSlug(title, threadKey string) string {
	if title == "" {
		return ""
	}

	// Extract thread ID from thread key
	parsed, err := keys.ParseKey(threadKey)
	if err != nil {
		// Fallback: use threadKey as-is if parsing fails
		return slugify(title) + "-" + threadKey
	}

	threadID := parsed.ThreadTS
	return slugify(title) + "-" + threadID
}

// slugify converts a string to a URL-friendly slug
func slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace spaces and special characters with hyphens
	re := regexp.MustCompile(`[^\w\s-]`)
	s = re.ReplaceAllString(s, "")

	// Replace spaces with hyphens
	re = regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, "-")

	// Remove consecutive hyphens
	re = regexp.MustCompile(`-+`)
	s = re.ReplaceAllString(s, "-")

	// Remove leading/trailing hyphens
	s = strings.Trim(s, "-")

	// Remove non-alphanumeric characters except hyphens
	var result strings.Builder
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			result.WriteRune(r)
		}
	}

	return result.String()
}
