package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"progressdb/pkg/ingest/apply"
	qpkg "progressdb/pkg/ingest/queue"
	"progressdb/pkg/state/logger"
)

// FailedOp represents a failed compute operation for recovery
type FailedOp struct {
	Timestamp time.Time         `json:"timestamp"`
	Key       string            `json:"key"`      // unique key
	Op        *qpkg.QueueOp     `json:"op"`       // original operation
	Error     string            `json:"error"`    // error message
	Retries   int               `json:"retries"`  // retry count
	Metadata  map[string]string `json:"metadata"` // additional context
}

// FailedOpWriter handles writing failed operations to recovery files
type FailedOpWriter struct {
	mu          sync.Mutex
	basePath    string
	current     *os.File
	currentDate string
}

// NewFailedOpWriter creates a new failed operation writer
func NewFailedOpWriter(basePath string) *FailedOpWriter {
	return &FailedOpWriter{
		basePath: basePath,
	}
}

// WriteFailedOp writes a failed operation to the recovery file
func (fw *FailedOpWriter) WriteFailedOp(op *qpkg.QueueOp, err error) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// ensure failed_ops directory exists
	if err := os.MkdirAll(fw.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create failed_ops directory: %w", err)
	}

	// rotate files daily
	date := time.Now().Format("2006-01-02")
	if fw.currentDate != date || fw.current == nil {
		if fw.current != nil {
			fw.current.Close()
		}

		filename := fmt.Sprintf("failed_ops_%s.jsonl", date)
		filepath := filepath.Join(fw.basePath, filename)

		file, openErr := os.OpenFile(filepath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if openErr != nil {
			return fmt.Errorf("failed to open failed_ops file: %w", openErr)
		}

		fw.current = file
		fw.currentDate = date
	}

	// extract keys using apply functions
	var key, threadKey string
	switch op.Handler {
	case qpkg.HandlerMessageCreate, qpkg.HandlerMessageUpdate, qpkg.HandlerMessageDelete:
		key = apply.ExtractMKey(op)
		threadKey = apply.ExtractTKey(op)
	default:
		key = apply.ExtractTKey(op)
		threadKey = apply.ExtractTKey(op)
	}

	// create failed operation record
	failedOp := FailedOp{
		Timestamp: time.Now(),
		Key:       fmt.Sprintf("%s_%d", key, time.Now().UnixNano()),
		Op:        op,
		Error:     err.Error(),
		Retries:   0,
		Metadata: map[string]string{
			"handler": string(op.Handler),
			"thread":  threadKey,
		},
	}

	// marshal and write
	data, marshalErr := json.Marshal(failedOp)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal failed op: %w", marshalErr)
	}

	if _, writeErr := fw.current.Write(append(data, '\n')); writeErr != nil {
		return fmt.Errorf("failed to write failed op: %w", writeErr)
	}

	logger.Error("failed_op_written", "id", failedOp.Key, "error", err, "handler", op.Handler)
	return nil
}

// Close closes the current failed ops file
func (fw *FailedOpWriter) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.current != nil {
		return fw.current.Close()
	}
	return nil
}
