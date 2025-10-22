package api

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/api/auth"
	"progressdb/pkg/api/router"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	message_store "progressdb/pkg/store/messages"
	thread_store "progressdb/pkg/store/threads"

	"github.com/valyala/fasthttp"
)

// ValidationResult represents the result of a validation operation
type ValidationResult struct {
	Success bool
	Error   *auth.AuthorResolutionError
	Data    interface{}
}

// ValidateThread validates thread existence, ownership, and deletion status
func ValidateThread(threadID, author string, requireOwnership bool) (*models.Thread, *auth.AuthorResolutionError) {
	// Check if thread is in user's deleted threads index
	if author != "" {
		deletedThreads, err := index.GetDeletedThreads(author)
		if err != nil {
			return nil, &auth.AuthorResolutionError{
				Type:    "index_error",
				Message: "failed to check deleted threads",
				Code:    fasthttp.StatusInternalServerError,
			}
		}
		for _, deletedID := range deletedThreads {
			if deletedID == threadID {
				return nil, &auth.AuthorResolutionError{
					Type:    "thread_not_found",
					Message: "thread not found",
					Code:    fasthttp.StatusNotFound,
				}
			}
		}
	}

	// Get thread from store
	stored, err := thread_store.GetThread(threadID)
	if err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "thread_not_found",
			Message: "thread not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	// Parse thread
	var thread models.Thread
	if err := json.Unmarshal([]byte(stored), &thread); err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "parse_error",
			Message: "failed to parse thread",
			Code:    fasthttp.StatusInternalServerError,
		}
	}

	// Check if thread is deleted
	if thread.Deleted {
		return nil, &auth.AuthorResolutionError{
			Type:    "thread_not_found",
			Message: "thread not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	// Check ownership if required
	if requireOwnership && author != "" && thread.Author != author {
		return nil, &auth.AuthorResolutionError{
			Type:    "forbidden",
			Message: "author does not match",
			Code:    fasthttp.StatusForbidden,
		}
	}

	return &thread, nil
}

// ValidateMessage validates message existence, ownership, and deletion status
func ValidateMessage(messageID, author string, requireOwnership bool) (*models.Message, *auth.AuthorResolutionError) {
	// Check if message is in user's deleted messages index
	if author != "" {
		deletedMessages, err := index.GetDeletedMessages(author)
		if err != nil {
			return nil, &auth.AuthorResolutionError{
				Type:    "index_error",
				Message: "failed to check deleted messages",
				Code:    fasthttp.StatusInternalServerError,
			}
		}
		for _, deletedID := range deletedMessages {
			if deletedID == messageID {
				return nil, &auth.AuthorResolutionError{
					Type:    "message_not_found",
					Message: "message not found",
					Code:    fasthttp.StatusNotFound,
				}
			}
		}
	}

	// Get message from store
	stored, err := message_store.GetLatestMessage(messageID)
	if err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "message_not_found",
			Message: "message not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	// Parse message
	var message models.Message
	if err := json.Unmarshal([]byte(stored), &message); err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "parse_error",
			Message: "failed to parse message",
			Code:    fasthttp.StatusInternalServerError,
		}
	}

	// Check if message is deleted
	if message.Deleted {
		return nil, &auth.AuthorResolutionError{
			Type:    "message_not_found",
			Message: "message not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	// Check ownership if required
	if requireOwnership && author != "" && message.Author != author {
		return nil, &auth.AuthorResolutionError{
			Type:    "forbidden",
			Message: "author does not match",
			Code:    fasthttp.StatusForbidden,
		}
	}

	return &message, nil
}

// ValidateAuthor resolves and validates author from request
func ValidateAuthor(ctx *fasthttp.RequestCtx, bodyAuthor string) (string, *auth.AuthorResolutionError) {
	author, err := auth.ResolveAuthorFromRequestFast(ctx, bodyAuthor)
	if err != nil {
		return "", err
	}
	return author, nil
}

// ValidateMessageThreadRelationship validates that a message belongs to a specific thread
func ValidateMessageThreadRelationship(message *models.Message, threadID string) *auth.AuthorResolutionError {
	if threadID != "" && message.Thread != threadID {
		return &auth.AuthorResolutionError{
			Type:    "not_found",
			Message: "message not found in thread",
			Code:    fasthttp.StatusNotFound,
		}
	}
	return nil
}

// WriteValidationError writes a validation error response
func WriteValidationError(ctx *fasthttp.RequestCtx, err *auth.AuthorResolutionError) {
	router.WriteJSONError(ctx, err.Code, err.Message)
}

// ValidateUserID performs basic validation (max 36 chars)
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
