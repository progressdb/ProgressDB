package router

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fasthttp"
)

// ValidateCreateThreadRequest validates thread creation payload.
func ValidateCreateThreadRequest(ctx *fasthttp.RequestCtx) error {
	body := ctx.PostBody()
	if len(body) == 0 {
		return fmt.Errorf("request body is required")
	}

	// Parse JSON to check fields (simple check)
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if title, ok := payload["title"].(string); !ok || title == "" {
		return fmt.Errorf("title is required and must be a non-empty string")
	}

	if author, ok := payload["author"].(string); !ok || author == "" {
		return fmt.Errorf("author is required and must be a non-empty string")
	}

	return nil
}

// ValidateCreateMessageRequest validates message creation payload.
func ValidateCreateMessageRequest(ctx *fasthttp.RequestCtx) error {
	body := ctx.PostBody()
	if len(body) == 0 {
		return fmt.Errorf("request body is required")
	}

	// Parse JSON to check fields
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if _, exists := payload["body"]; !exists {
		return fmt.Errorf("body is required")
	}

	// Body can be any type, so no further check

	return nil
}
