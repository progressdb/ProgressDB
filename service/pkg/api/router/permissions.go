package router

import (
	"errors"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/keys"
)

var (
	ErrThreadDeleted  = errors.New("thread not found")
	ErrMessageDeleted = errors.New("message not found")
)

// ValidateThreadNotDeleted returns an error if thread is deleted
func ValidateThreadNotDeleted(threadKey string) error {
	deleteMarkerKey := keys.GenSoftDeleteMarkerKey(threadKey)
	_, err := indexdb.GetKey(deleteMarkerKey)
	if err == nil {
		return ErrThreadDeleted
	}
	return nil
}

// ValidateMessageNotDeleted returns an error if message is deleted
func ValidateMessageNotDeleted(messageKey string) error {
	deleteMarkerKey := keys.GenSoftDeleteMarkerKey(messageKey)
	_, err := indexdb.GetKey(deleteMarkerKey)
	if err == nil {
		return ErrMessageDeleted
	}
	return nil
}

// ValidateThreadAndMessageNotDeleted validates both thread and message deletion status
func ValidateThreadAndMessageNotDeleted(threadKey, messageKey string) error {
	if err := ValidateThreadNotDeleted(threadKey); err != nil {
		return err
	}

	if err := ValidateMessageNotDeleted(messageKey); err != nil {
		return err
	}

	return nil
}

// HandleDeletedError writes appropriate HTTP response for deletion errors
func HandleDeletedError(ctx *fasthttp.RequestCtx, err error) bool {
	if errors.Is(err, ErrThreadDeleted) || errors.Is(err, ErrMessageDeleted) {
		WriteJSONError(ctx, fasthttp.StatusNotFound, err.Error())
		return true
	}
	return false
}
