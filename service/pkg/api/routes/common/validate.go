package common

import (
	"encoding/json"
	"fmt"

	"progressdb/pkg/api/auth"
	"progressdb/pkg/api/router"
	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	message_store "progressdb/pkg/store/features/messages"
	thread_store "progressdb/pkg/store/features/threads"

	"github.com/valyala/fasthttp"
)

type ValidationResult struct {
	Success bool
	Error   *auth.AuthorResolutionError
	Data    interface{}
}

// ValidateReadThread validates read access to a thread
func ValidateReadThread(threadID, author string, requireOwnership bool) (*models.Thread, *auth.AuthorResolutionError) {
	if author != "" {
		isDeleted, err := index.IsSoftDeleted(threadID)
		if err != nil {
			return nil, &auth.AuthorResolutionError{
				Type:    "index_error",
				Message: "failed to check deleted threads",
				Code:    fasthttp.StatusInternalServerError,
			}
		}
		if isDeleted {
			return nil, &auth.AuthorResolutionError{
				Type:    "thread_not_found",
				Message: "thread not found",
				Code:    fasthttp.StatusNotFound,
			}
		}
	}

	stored, err := thread_store.GetThread(threadID)
	if err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "thread_not_found",
			Message: "thread not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	var thread models.Thread
	if err := json.Unmarshal([]byte(stored), &thread); err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "parse_error",
			Message: "failed to parse thread",
			Code:    fasthttp.StatusInternalServerError,
		}
	}

	if thread.Deleted {
		return nil, &auth.AuthorResolutionError{
			Type:    "thread_not_found",
			Message: "thread not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	if requireOwnership && author != "" && thread.Author != author {
		return nil, &auth.AuthorResolutionError{
			Type:    "forbidden",
			Message: "author does not match",
			Code:    fasthttp.StatusForbidden,
		}
	}

	return &thread, nil
}

func ValidateReadMessage(messageID, author string, requireOwnership bool) (*models.Message, *auth.AuthorResolutionError) {
	if author != "" {
		isDeleted, err := index.IsSoftDeleted(messageID)
		if err != nil {
			return nil, &auth.AuthorResolutionError{
				Type:    "index_error",
				Message: "failed to check deleted messages",
				Code:    fasthttp.StatusInternalServerError,
			}
		}
		if isDeleted {
			return nil, &auth.AuthorResolutionError{
				Type:    "message_not_found",
				Message: "message not found",
				Code:    fasthttp.StatusNotFound,
			}
		}
	}

	stored, err := message_store.GetLatestMessage(messageID)
	if err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "message_not_found",
			Message: "message not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	var message models.Message
	if err := json.Unmarshal([]byte(stored), &message); err != nil {
		return nil, &auth.AuthorResolutionError{
			Type:    "parse_error",
			Message: "failed to parse message",
			Code:    fasthttp.StatusInternalServerError,
		}
	}

	if message.Deleted {
		return nil, &auth.AuthorResolutionError{
			Type:    "message_not_found",
			Message: "message not found",
			Code:    fasthttp.StatusNotFound,
		}
	}

	if requireOwnership && author != "" && message.Author != author {
		return nil, &auth.AuthorResolutionError{
			Type:    "forbidden",
			Message: "author does not match",
			Code:    fasthttp.StatusForbidden,
		}
	}

	return &message, nil
}

func ValidateAuthor(ctx *fasthttp.RequestCtx, bodyAuthor string) (string, *auth.AuthorResolutionError) {
	author, err := auth.ResolveAuthorFromRequestFast(ctx, bodyAuthor)
	if err != nil {
		return "", err
	}
	return author, nil
}

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

func WriteValidationError(ctx *fasthttp.RequestCtx, err *auth.AuthorResolutionError) {
	router.WriteJSONError(ctx, err.Code, err.Message)
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
