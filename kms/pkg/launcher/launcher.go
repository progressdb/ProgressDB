package launcher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	MasterKeyHex string
	Socket       string
	DataDir      string
	LogFile      string
}

// ChildHandle represents a spawned KMS process
type ChildHandle struct {
	Cmd     *exec.Cmd
	Stdout  bytes.Buffer
	Stderr  bytes.Buffer
	done    chan error
	cfgPath string
}

// CreateSecureConfigFile writes a self-contained YAML config with 0600 perms
func CreateSecureConfigFile(cfg *Config, dir string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("nil cfg")
	}
	if dir == "" {
		dir = cfg.DataDir
	}
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "kms-config.yaml")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return "", err
	}
	defer f.Close()
	out := map[string]string{
		"master_key_hex": cfg.MasterKeyHex,
	}
	if cfg.Socket != "" {
		out["socket"] = cfg.Socket
	}
	if cfg.DataDir != "" {
		out["data_dir"] = cfg.DataDir
	}
	if cfg.LogFile != "" {
		out["log_file"] = cfg.LogFile
	}
	enc := yaml.NewEncoder(f)
	if err := enc.Encode(out); err != nil {
		return "", err
	}
	enc.Close()
	return path, nil
}

// StartChild starts the binary with --config <cfgPath>. If ln is non-nil it
// will be passed to the child via ExtraFiles (inherited as fd 3).
func StartChild(ctx context.Context, bin string, cfgPath string, ln *net.UnixListener) (*ChildHandle, error) {
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("binary not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, bin, "--config", cfgPath)
	// minimal env
	cmd.Env = os.Environ()
	if ln != nil {
		f, err := ln.File()
		if err != nil {
			return nil, err
		}
		cmd.ExtraFiles = []*os.File{f}
	}
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start child: %w", err)
	}
	h := &ChildHandle{Cmd: cmd, done: make(chan error, 1), cfgPath: cfgPath}
	go func() { _, _ = io.Copy(io.MultiWriter(os.Stdout, &h.Stdout), stdoutPipe) }()
	go func() { _, _ = io.Copy(io.MultiWriter(os.Stderr, &h.Stderr), stderrPipe) }()
	go func() { h.done <- cmd.Wait() }()
	// readiness: if ln==nil, wait for socket path in cfg
	if ln == nil {
		// parse socket from config
		data, err := os.ReadFile(cfgPath)
		if err == nil {
			var m map[string]string
			_ = yaml.Unmarshal(data, &m)
			if sp, ok := m["socket"]; ok && sp != "" {
				// wait up to 10s
				deadline := time.Now().Add(10 * time.Second)
				for {
					if _, err := os.Stat(sp); err == nil {
						break
					}
					if time.Now().After(deadline) {
						cmd.Process.Kill()
						return nil, fmt.Errorf("socket %s not ready", sp)
					}
					select {
					case <-ctx.Done():
						cmd.Process.Kill()
						return nil, ctx.Err()
					case <-time.After(100 * time.Millisecond):
					}
				}
			}
		}
	}
	return h, nil
}

// Stop tries graceful shutdown then kills the child after timeout
func (h *ChildHandle) Stop(timeout time.Duration) error {
	if h == nil || h.Cmd == nil || h.Cmd.Process == nil {
		return nil
	}
	_ = h.Cmd.Process.Signal(syscall.SIGTERM)
	select {
	case err := <-h.done:
		return err
	case <-time.After(timeout):
		_ = h.Cmd.Process.Kill()
		return fmt.Errorf("killed child after timeout")
	}
}
