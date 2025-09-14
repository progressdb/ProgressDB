package kms

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "syscall"
    "time"
)

// CmdHandle represents a spawned KMS child process and provides control helpers.
type CmdHandle struct {
    Cmd *exec.Cmd
    // captured output from child process
    Stdout bytes.Buffer
    Stderr bytes.Buffer
    // done receives the result of cmd.Wait()
    done chan error
}

// StartChild starts the given binary with args and waits for a readiness file
// (socketPath) to appear. It returns a CmdHandle that the caller may Stop.
// The function keeps environment minimal and does not pass secrets on the
// command-line. Caller is responsible for cleanup of socketPath if needed.
func StartChild(ctx context.Context, binary string, args []string, socketPath string, uid, gid uint32, readyTimeout time.Duration, env map[string]string) (*CmdHandle, error) {
	if _, err := os.Stat(binary); err != nil {
		return nil, fmt.Errorf("binary not found: %w", err)
	}

	// Ensure socket dir exists with restrictive perms
	if socketPath != "" {
		dir := filepath.Dir(socketPath)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, fmt.Errorf("mkdir socket dir: %w", err)
		}
	}

    cmd := exec.CommandContext(ctx, binary, args...)
    // Minimal environment plus provided env variables
    baseEnv := []string{"PATH=/usr/bin:/bin"}
    for k, v := range env {
        baseEnv = append(baseEnv, k+"="+v)
    }
    cmd.Env = baseEnv

    // Drop privileges for the child when supported
    cmd.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{Uid: uid, Gid: gid}}

    // Redirect stdout/stderr so server logs capture child logs and we can
    // capture them for startup diagnostics.
    stdoutPipe, _ := cmd.StdoutPipe()
    stderrPipe, _ := cmd.StderrPipe()

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("start child: %w", err)
    }

    h := &CmdHandle{Cmd: cmd, done: make(chan error, 1)}

    // Copy stdout/stderr to both os.Stdout/os.Stderr and internal buffers
    go func() {
        mw := io.MultiWriter(os.Stdout, &h.Stdout)
        _, _ = io.Copy(mw, stdoutPipe)
    }()
    go func() {
        mw := io.MultiWriter(os.Stderr, &h.Stderr)
        _, _ = io.Copy(mw, stderrPipe)
    }()

    // start a goroutine to reap the child process and report its exit
    go func() { h.done <- cmd.Wait() }()

    // wait for readiness
    deadline := time.Now().Add(readyTimeout)
    for {
        if socketPath == "" {
            // no readiness check configured; assume started
            break
        }
        if _, err := os.Stat(socketPath); err == nil {
            break
        }
        if time.Now().After(deadline) {
            // try to kill process
            _ = cmd.Process.Kill()
            // include captured output for diagnostics
            out := h.Stdout.String()
            errout := h.Stderr.String()
            return nil, fmt.Errorf("kms readiness timeout, socket %s not available; stdout=%q stderr=%q", socketPath, out, errout)
        }
        select {
        case <-ctx.Done():
            _ = cmd.Process.Kill()
            return nil, ctx.Err()
        case <-time.After(100 * time.Millisecond):
        }
    }

    return h, nil
}

// Stop gracefully stops the child process and waits for it to exit.
func (h *CmdHandle) Stop(timeout time.Duration) error {
    if h == nil || h.Cmd == nil || h.Cmd.Process == nil {
        return nil
    }
    // try graceful termination
    _ = h.Cmd.Process.Signal(syscall.SIGTERM)
    select {
    case <-time.After(timeout):
        _ = h.Cmd.Process.Kill()
        // wait for reaper to finish
        select {
        case err := <-h.done:
            return fmt.Errorf("child killed after timeout: %v", err)
        default:
            return fmt.Errorf("child did not exit, killed")
        }
    case err := <-h.done:
        return err
    }
}
