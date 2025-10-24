package router

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

func (vr *ValidationResult) AddError(field, message string) {
	vr.Valid = false
	vr.Errors = append(vr.Errors, ValidationError{Field: field, Message: message})
}

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

type ThreadPayload struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Slug   string `json:"slug,omitempty"`
}

type ThreadUpdatePayload struct {
	Title *string `json:"title,omitempty"`
	Slug  *string `json:"slug,omitempty"`
}

type MessagePayload struct {
	Body interface{} `json:"body"`
}

type ReactionPayload struct {
	Reaction string `json:"reaction"`
	Identity string `json:"identity"`
}

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

func ValidateUpdateThreadRequest(ctx *fasthttp.RequestCtx) error {
	body := ctx.PostBody()
	if len(body) == 0 {
		return &ValidationError{Field: "body", Message: "request body is required"}
	}

	var payload ThreadUpdatePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return &ValidationError{Field: "body", Message: fmt.Sprintf("invalid JSON: %v", err)}
	}

	// For updates, all fields are optional, but if provided, validate them
	result := &ValidationResult{Valid: true}

	if payload.Title != nil && *payload.Title == "" {
		result.AddError("title", "title must be a non-empty string if provided")
	}

	if payload.Slug != nil && *payload.Slug == "" {
		result.AddError("slug", "slug must be a non-empty string if provided")
	}

	if !result.Valid {
		return result
	}
	return nil
}

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
