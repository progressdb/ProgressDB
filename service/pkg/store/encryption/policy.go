package encryption

import (
	"fmt"
	"strings"
)

type fieldRule struct {
	segments []string
}

var fieldRules []fieldRule

func SetEncryptionFieldPolicy(fields []string) error {
	fieldRules = fieldRules[:0]
	for _, p := range fields {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		segments := strings.Split(p, ".")
		if len(segments) == 0 || segments[0] != "body" {
			return fmt.Errorf("encryption field path must start with 'body': %q", p)
		}
		fieldRules = append(fieldRules, fieldRule{segments: segments})
	}
	return nil
}

func EncryptionHasFieldPolicy() bool {
	return len(fieldRules) > 0
}
