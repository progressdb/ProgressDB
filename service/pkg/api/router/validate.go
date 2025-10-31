package router

import (
	"fmt"
	"progressdb/pkg/models"
	"strings"
)

func ValidateAllFieldsNonEmpty(p interface{}) error {
	var errors []string

	switch v := p.(type) {
	case *models.ThreadUpdatePartial:
		if v == nil {
			errors = append(errors, "ThreadUpdatePartial cannot be nil")
		} else {
			if v.Key == "" {
				errors = append(errors, "key: cannot be empty")
			}
			if v.UpdatedTS == 0 {
				errors = append(errors, "updated_ts: cannot be zero")
			}
			if v.Title == "" {
				errors = append(errors, "title: cannot be empty")
			}
			if v.Slug == "" {
				errors = append(errors, "slug: cannot be empty")
			}
		}
	case *models.MessageUpdatePartial:
		if v == nil {
			errors = append(errors, "MessageUpdatePartial cannot be nil")
		} else {
			if v.Key == "" {
				errors = append(errors, "key: cannot be empty")
			}
			if v.Thread == "" {
				errors = append(errors, "thread: cannot be empty")
			}
			if v.Body == nil {
				errors = append(errors, "body: cannot be empty")
			}
			if v.TS == 0 {
				errors = append(errors, "ts: cannot be zero")
			}
		}
	case *models.ThreadDeletePartial:
		if v == nil {
			errors = append(errors, "ThreadDeletePartial cannot be nil")
		} else {
			if v.Key == "" {
				errors = append(errors, "key: cannot be empty")
			}
			if v.TS == 0 {
				errors = append(errors, "ts: cannot be zero")
			}
		}
	case *models.MessageDeletePartial:
		if v == nil {
			errors = append(errors, "MessageDeletePartial cannot be nil")
		} else {
			if v.Key == "" {
				errors = append(errors, "key: cannot be empty")
			}
			if !v.Deleted {
				errors = append(errors, "deleted: must be true")
			}
			if v.TS == 0 {
				errors = append(errors, "ts: cannot be zero")
			}
			if v.Thread == "" {
				errors = append(errors, "thread: cannot be empty")
			}
			if v.Author == "" {
				errors = append(errors, "author: cannot be empty")
			}
		}
	case *models.Message:
		if v == nil {
			errors = append(errors, "Message cannot be nil")
		} else {
			if v.Key == "" {
				errors = append(errors, "key: cannot be empty")
			}
			if v.Thread == "" {
				errors = append(errors, "thread: cannot be empty")
			}
			if v.Author == "" {
				errors = append(errors, "author: cannot be empty")
			}
			if v.Body == nil {
				errors = append(errors, "body: cannot be empty")
			}
			if v.TS == 0 {
				errors = append(errors, "ts: cannot be zero")
			}
		}
	case *models.Thread:
		if v == nil {
			errors = append(errors, "Thread cannot be nil")
		} else {
			if v.Key == "" {
				errors = append(errors, "key: cannot be empty")
			}
			if v.Author == "" {
				errors = append(errors, "author: cannot be empty")
			}
			if v.Title == "" {
				errors = append(errors, "title: cannot be empty")
			}
			if v.CreatedTS == 0 {
				errors = append(errors, "created_ts: cannot be zero")
			}
		}
	default:
		errors = append(errors, "unsupported payload type for validation")
	}
	if len(errors) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(errors, "; "))
	}
	return nil
}
