package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	qpkg "progressdb/pkg/ingest/queue"
	"progressdb/pkg/state/logger"
)

type FailedOp struct {
	Timestamp time.Time         `json:"timestamp"`
	Key       string            `json:"key"`
	Op        *qpkg.QueueOp     `json:"op"`
	Error     string            `json:"error"`
	Retries   int               `json:"retries"`
	Metadata  map[string]string `json:"metadata"`
}

type FailedOpWriter struct {
	mu          sync.Mutex
	basePath    string
	current     *os.File
	currentDate string
}

func NewFailedOpWriter(basePath string) *FailedOpWriter {
	return &FailedOpWriter{
		basePath: basePath,
	}
}

func (fw *FailedOpWriter) WriteFailedOp(op *qpkg.QueueOp, err error) error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if err := os.MkdirAll(fw.basePath, 0755); err != nil {
		return fmt.Errorf("failed to create failed_ops directory: %w", err)
	}

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

	failedOp := FailedOp{
		Timestamp: time.Now(),
		Key:       fmt.Sprintf("%s_%d", op.Extras.ReqID, time.Now().UnixNano()),
		Op:        op,
		Error:     err.Error(),
		Retries:   0,
		Metadata: map[string]string{
			"handler": string(op.Handler),
		},
	}

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

func (fw *FailedOpWriter) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	if fw.current != nil {
		return fw.current.Close()
	}
	return nil
}

// Crash writes a crash dump to the crash folder with diagnostics.
func Crash(reason string, err error) (string, error) {
	crashDir := PathsVar.Crash
	if crashDir == "" {
		return "", fmt.Errorf("crash path not initialized")
	}

	if e := os.MkdirAll(crashDir, 0o700); e != nil {
		return "", fmt.Errorf("failed to create crash dir: %w", e)
	}

	ts := time.Now().UnixNano()
	dumpName := fmt.Sprintf("crash-%d.log", ts)
	dumpPath := filepath.Join(crashDir, dumpName)

	f, ferr := os.Create(dumpPath)
	if ferr != nil {
		return "", fmt.Errorf("failed to create crash dump file: %w", ferr)
	}
	defer f.Close()

	fmt.Fprintf(f, "time: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, "reason: %s\n", reason)
	if err != nil {
		fmt.Fprintf(f, "error: %v\n", err)
	}
	fmt.Fprintf(f, "\n--- environ ---\n")
	for _, e := range os.Environ() {
		fmt.Fprintln(f, e)
	}
	fmt.Fprintf(f, "\n--- goroutine stacks ---\n")
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	f.Write(buf[:n])

	logger.Error("crash_dump_written", "path", dumpPath, "reason", reason, "error", err)
	return dumpPath, nil
}
