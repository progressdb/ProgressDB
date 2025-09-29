package tests

import (
    "os"
    "path/filepath"
    "testing"

    "progressdb/pkg/config"
)

func TestConfig_LoadAndResolve(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "cfg.yaml")
    content := []byte("server:\n  address: 127.0.0.1\n  port: 9090\nlogging:\n  level: debug\n")
    if err := os.WriteFile(p, content, 0o600); err != nil {
        t.Fatalf("failed to write config file: %v", err)
    }
    c, err := config.Load(p)
    if err != nil {
        t.Fatalf("config.Load failed: %v", err)
    }
    if c.Server.Port != 9090 {
        t.Fatalf("expected port 9090 got %d", c.Server.Port)
    }

    // ResolveConfigPath prefers env var when flag not set
    os.Setenv("PROGRESSDB_SERVER_CONFIG", p)
    defer os.Unsetenv("PROGRESSDB_SERVER_CONFIG")
    got := config.ResolveConfigPath("/nope", false)
    if got != p {
        t.Fatalf("ResolveConfigPath expected %q got %q", p, got)
    }
}

