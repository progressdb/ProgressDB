package router

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"
)

// ValidationError represents a validation error with field context
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

// ValidationResult represents the result of validation
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// AddError adds a validation error to the result
func (vr *ValidationResult) AddError(field, message string) {
	vr.Valid = false
	vr.Errors = append(vr.Errors, ValidationError{Field: field, Message: message})
}

// Error implements the error interface for ValidationResult
func (vr *ValidationResult) Error() string {
	if vr.Valid {
		return ""
	}

	msg := "validation failed:"
	for _, err := range vr.Errors {
		msg += fmt.Sprintf(" %s;", err.Error())
	}
	return msg
}

// ThreadPayload represents a validated thread creation payload
type ThreadPayload struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Slug   string `json:"slug,omitempty"`
}

// MessagePayload represents a validated message creation payload
type MessagePayload struct {
	Body interface{} `json:"body"`
}

// ReactionPayload represents a validated reaction payload
type ReactionPayload struct {
	Reaction string `json:"reaction"`
	Identity string `json:"identity"`
}

// ValidateCreateThreadRequest validates thread creation payload.
func ValidateCreateThreadRequest(ctx *fasthttp.RequestCtx) error {
	body := ctx.PostBody()
	if len(body) == 0 {
		return &ValidationError{Field: "body", Message: "request body is required"}
	}

	var payload ThreadPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return &ValidationError{Field: "body", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}

	result := &ValidationResult{Valid: true}

	if payload.Title == "" {
		result.AddError("title", "title is required and must be a non-empty string")
	}

	if payload.Author == "" {
		result.AddError("author", "author is required and must be a non-empty string")
	}

	if !result.Valid {
		return result
	}

	return nil
}

// ValidateCreateMessageRequest validates message creation payload.
func ValidateCreateMessageRequest(ctx *fasthttp.RequestCtx) error {
	body := ctx.PostBody()
	if len(body) == 0 {
		return &ValidationError{Field: "body", Message: "request body is required"}
	}

	var payload MessagePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return &ValidationError{Field: "body", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}

	result := &ValidationResult{Valid: true}

	if payload.Body == nil {
		result.AddError("body", "body is required")
	}

	if !result.Valid {
		return result
	}

	return nil
}

// ValidateReactionRequest validates reaction payload.
func ValidateReactionRequest(ctx *fasthttp.RequestCtx) error {
	body := ctx.PostBody()
	if len(body) == 0 {
		return &ValidationError{Field: "body", Message: "request body is required"}
	}

	var payload ReactionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return &ValidationError{Field: "body", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}

	result := &ValidationResult{Valid: true}

	if payload.Reaction == "" {
		result.AddError("reaction", "reaction is required")
	}

	if payload.Identity == "" {
		result.AddError("identity", "identity is required")
	}

	if !result.Valid {
		return result
	}

	return nil
}
