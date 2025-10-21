package ingest

import (
	"errors"
	"strings"

	"progressdb/pkg/models"
	"progressdb/pkg/telemetry"
)

func ValidateMessage(m models.Message) error {
	tr := telemetry.Track("validation.validate_message")
	defer tr.Finish()

	var errs []string
	if m.Body == nil {
		errs = append(errs, "body is required")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func ValidateThread(th models.Thread) error {
	tr := telemetry.Track("validation.validate_thread")
	defer tr.Finish()

	var errs []string
	if th.ID == "" {
		errs = append(errs, "id is required")
	}
	if th.Title == "" {
		errs = append(errs, "title is required")
	}
	if th.Author == "" {
		errs = append(errs, "author is required")
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
