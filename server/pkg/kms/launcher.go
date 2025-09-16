package kms

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

type LauncherConfig struct {
	MasterKeyHex string
	Socket       string
	DataDir      string
	LogFile      string
}

type LauncherHandle struct {
	Cmd     *exec.Cmd
	Stdout  bytes.Buffer
	Stderr  bytes.Buffer
	done    chan error
	cfgPath string
}

func CreateSecureConfigFile(cfg *LauncherConfig, dir string) (string, error) {
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

func StartChildLauncher(ctx context.Context, bin string, cfgPath string, ln *net.UnixListener) (*LauncherHandle, error) {
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("binary not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, bin, "--config", cfgPath)
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
	h := &LauncherHandle{Cmd: cmd, done: make(chan error, 1), cfgPath: cfgPath}
	go func() { _, _ = io.Copy(io.MultiWriter(os.Stdout, &h.Stdout), stdoutPipe) }()
	go func() { _, _ = io.Copy(io.MultiWriter(os.Stderr, &h.Stderr), stderrPipe) }()
	go func() { h.done <- cmd.Wait() }()
	if ln == nil {
		data, err := os.ReadFile(cfgPath)
		if err == nil {
			var m map[string]string
			_ = yaml.Unmarshal(data, &m)
			if sp, ok := m["socket"]; ok && sp != "" {
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

func (h *LauncherHandle) Stop(timeout time.Duration) error {
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
