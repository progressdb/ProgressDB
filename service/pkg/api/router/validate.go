package router

import (
	"encoding/json"
	"fmt"
	"strings"

	"progressdb/pkg/api/utils"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/indexdb"
	message_store "progressdb/pkg/store/features/messages"
	thread_store "progressdb/pkg/store/features/threads"

	"github.com/valyala/fasthttp"
)

type AuthorResolutionError struct {
	Type    string
	Message string
	Code    int
}

func (e *AuthorResolutionError) Error() string {
	return e.Message
}

var (
	ErrAuthorRequired     = &AuthorResolutionError{"author_required", "author required", fasthttp.StatusBadRequest}
	ErrAuthorTooLong      = &AuthorResolutionError{"author_too_long", "author too long", fasthttp.StatusBadRequest}
	ErrInvalidSignature   = &AuthorResolutionError{"invalid_signature", "missing or invalid author signature", fasthttp.StatusUnauthorized}
	ErrAuthorMismatch     = &AuthorResolutionError{"author_mismatch", "author mismatch", fasthttp.StatusForbidden}
	ErrBackendMissingAuth = &AuthorResolutionError{"backend_missing_auth", "author required for backend requests", fasthttp.StatusBadRequest}
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
			if v.UpdatedTS == 0 {
				errors = append(errors, "updated_ts: cannot be zero")
			}
		}
	case *models.ThreadDeletePartial:
		if v == nil {
			errors = append(errors, "ThreadDeletePartial cannot be nil")
		} else {
			if v.Key == "" {
				errors = append(errors, "key: cannot be empty")
			}
			if v.UpdatedTS == 0 {
				errors = append(errors, "updated_ts: cannot be zero")
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
			if v.UpdatedTS == 0 {
				errors = append(errors, "updated_ts: cannot be zero")
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
			if v.UpdatedTS == 0 {
				errors = append(errors, "updated_ts: cannot be zero")
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

type ValidationResult struct {
	Success bool
	Error   *AuthorResolutionError
	Data    interface{}
}

// ValidateReadThread validates read access to a thread
func ValidateReadThread(threadKey, author string, requireOwnership bool) (*models.Thread, *AuthorResolutionError) {
	if author != "" {
		isDeleted, err := indexdb.IsSoftDeleted(threadKey)
		if err != nil {
			return nil, &AuthorResolutionError{
				Type:    "index_error",
				Message: "failed to check deleted threads",
				Code:    fasthttp.StatusInternalServerError,
			}
		}
		if isDeleted {
			return nil, &AuthorResolutionError{
				Type:    "thread_not_found",
				Message: "thread not found",
				Code:    fasthttp.StatusNotFound,
			}
		}
	}

	stored, err := thread_store.GetThreadData(threadKey)
	if err != nil {
		return nil, &AuthorResolutionError{
			Type:    "thread_not_found",
			Message: "thread not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	var thread models.Thread
	if err := json.Unmarshal([]byte(stored), &thread); err != nil {
		return nil, &AuthorResolutionError{
			Type:    "parse_error",
			Message: "failed to parse thread",
			Code:    fasthttp.StatusInternalServerError,
		}
	}

	if thread.Deleted {
		return nil, &AuthorResolutionError{
			Type:    "thread_not_found",
			Message: "thread not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	if requireOwnership && author != "" && thread.Author != author {
		return nil, &AuthorResolutionError{
			Type:    "forbidden",
			Message: "author does not match",
			Code:    fasthttp.StatusForbidden,
		}
	}

	return &thread, nil
}

func ValidateReadMessage(messageKey, author string, requireOwnership bool) (*models.Message, *AuthorResolutionError) {
	if author != "" {
		isDeleted, err := indexdb.IsSoftDeleted(messageKey)
		if err != nil {
			return nil, &AuthorResolutionError{
				Type:    "index_error",
				Message: "failed to check deleted messages",
				Code:    fasthttp.StatusInternalServerError,
			}
		}
		if isDeleted {
			return nil, &AuthorResolutionError{
				Type:    "message_not_found",
				Message: "message not found",
				Code:    fasthttp.StatusNotFound,
			}
		}
	}

	stored, err := message_store.GetMessageData(messageKey)
	if err != nil {
		return nil, &AuthorResolutionError{
			Type:    "message_not_found",
			Message: "message not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	var message models.Message
	if err := json.Unmarshal([]byte(stored), &message); err != nil {
		return nil, &AuthorResolutionError{
			Type:    "parse_error",
			Message: "failed to parse message",
			Code:    fasthttp.StatusInternalServerError,
		}
	}

	if message.Deleted {
		return nil, &AuthorResolutionError{
			Type:    "message_not_found",
			Message: "message not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	if requireOwnership && author != "" && message.Author != author {
		return nil, &AuthorResolutionError{
			Type:    "forbidden",
			Message: "author does not match",
			Code:    fasthttp.StatusForbidden,
		}
	}

	return &message, nil
}

func validateAuthor(a string) *AuthorResolutionError {
	if a == "" {
		return ErrAuthorRequired
	}
	if len(a) > 128 {
		return ErrAuthorTooLong
	}
	return nil
}

// extract author - depending on frontend or backend role
func ResolveAuthorFromRequestFast(ctx *fasthttp.RequestCtx, bodyAuthor string) (string, *AuthorResolutionError) {
	// signature-verified author from user value if present
	if v := ctx.UserValue("author"); v != nil {
		if id, ok := v.(string); ok && id != "" {
			if q := utils.GetQuery(ctx, "author"); q != "" && q != id {
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and query param", Code: fasthttp.StatusForbidden}
			}
			if h := utils.GetUserID(ctx); h != "" && h != id {
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and header", Code: fasthttp.StatusForbidden}
			}
			if bodyAuthor != "" && bodyAuthor != id {
				return "", &AuthorResolutionError{Type: "author_mismatch", Message: "author mismatch between signature and body author", Code: fasthttp.StatusForbidden}
			}
			ctx.Request.Header.Set("X-User-ID", id)
			return id, nil
		}
	}

	// no signature; allow backend to supply author via body/header/query
	role := utils.GetRole(ctx)
	if role == "backend" {
		if bodyAuthor != "" {
			if err := validateAuthor(bodyAuthor); err != nil {
				return "", err
			}
			ctx.Request.Header.Set("X-User-ID", bodyAuthor)
			return bodyAuthor, nil
		}
		if h := utils.GetUserID(ctx); h != "" {
			if err := validateAuthor(h); err != nil {
				return "", err
			}
			ctx.Request.Header.Set("X-User-ID", h)
			return h, nil
		}
		if q := utils.GetQuery(ctx, "author"); q != "" {
			if err := validateAuthor(q); err != nil {
				return "", err
			}
			ctx.Request.Header.Set("X-User-ID", q)
			return q, nil
		}
		return "", ErrBackendMissingAuth
	}

	// otherwise require signature
	return "", ErrInvalidSignature
}

func ValidateAuthor(ctx *fasthttp.RequestCtx, bodyAuthor string) (string, *AuthorResolutionError) {
	author, err := ResolveAuthorFromRequestFast(ctx, bodyAuthor)
	if err != nil {
		return "", err
	}
	return author, nil
}

func ValidateMessageThreadRelationship(message *models.Message, threadKey string) *AuthorResolutionError {
	if threadKey != "" && message.Thread != threadKey {
		return &AuthorResolutionError{
			Type:    "not_found",
			Message: "message not found in thread",
			Code:    fasthttp.StatusNotFound,
		}
	}
	return nil
}

func WriteValidationError(ctx *fasthttp.RequestCtx, err *AuthorResolutionError) {
	WriteJSONError(ctx, err.Code, err.Message)
}

func ValidateUserID(userID string) error {
	const maxLen = 36

	if len(userID) == 0 {
		return fmt.Errorf("user ID cannot be empty")
	}
	if len(userID) > maxLen {
		return fmt.Errorf("user ID too long (maximum 36 characters)")
	}
	for _, r := range userID {
		if r < 32 || r == 127 {
			return fmt.Errorf("user ID contains invalid control characters")
		}
	}
	return nil
}
