package keys

import (
	"errors"
	"fmt"
	"regexp"
)

var (
	// conservative ID validation: letters, digits, dot, underscore, dash
	// and a reasonable upper bound to protect DB key shapes.
	idRegexp = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)
)

// ValidateThreadID ensures a thread id is safe to embed in keys.
func ValidateThreadID(id string) error {
	if id == "" {
		return errors.New("thread id empty")
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("invalid thread id: %q", id)
	}
	return nil
}

// ValidateMsgID ensures a message id is safe to embed in keys.
func ValidateMsgID(id string) error {
	if id == "" {
		return errors.New("msg id empty")
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("invalid msg id: %q", id)
	}
	return nil
}

// ValidateUserID ensures a user id is safe to embed in keys.
func ValidateUserID(id string) error {
	if id == "" {
		return errors.New("user id empty")
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("invalid user id: %q", id)
	}
	return nil
}
