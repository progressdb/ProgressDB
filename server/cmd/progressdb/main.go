package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"progressdb/internal/app"
	"progressdb/pkg/config"
	"progressdb/pkg/logger"

	"github.com/joho/godotenv"
)

func main() {
	// set build metadata
	var (
		version   = "dev"
		commit    = "none"
		buildDate = "unknown"
	)

	// load .env file if present
	_ = godotenv.Load(".env")

	// initialize centralized logger
	logger.Init()
	defer logger.Sync()

	// after loading effective config below we will attach the audit file sink

	// parse config flags
	flags := config.ParseConfigFlags()

	// parse config file
	fileCfg, fileExists, err := config.ParseConfigFile(flags)
	if err != nil {
		log.Fatalf("failed to load config file: %v", err)
	}

	// parse config from environment variables
	envCfg, envRes := config.ParseConfigEnvs()

	// load effective config
	eff, err := config.LoadEffectiveConfig(flags, fileCfg, fileExists, envCfg, envRes)
	if err != nil {
		log.Fatalf("failed to build effective config: %v", err)
	}

	// Attach audit file sink to a fixed retention folder under DBPath.
	auditPath := filepath.Join(eff.DBPath, "retention")
	if err := logger.AttachAuditFileSink(auditPath); err != nil {
		logger.Error("attach_audit_sink_failed", "error", err)
	}

	// initialize app
	app, err := app.New(eff, version, commit, buildDate)
	if err != nil {
		log.Fatalf("failed to initialize app: %v", err)
	}

	// set up context and signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigc
		log.Printf("signal received: %v, shutdown requested", s)
		cancel()
	}()

	// Monitor SIGPIPE and print diagnostics if it occurs. Some test harness
	// environments may deliver SIGPIPE when writing to closed pipes; catching
	// it here and dumping goroutine stacks helps diagnose abrupt exits.
	sigpipe := make(chan os.Signal, 1)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	go func() {
		s := <-sigpipe
		log.Printf("signal received: %v (SIGPIPE) - dumping goroutine stacks", s)
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		log.Printf("=== goroutine stack dump ===\n%s\n=== end goroutine stack dump ===", string(buf[:n]))
		// cancel context to trigger graceful shutdown
		cancel()
	}()

	// run the app
	if err := app.Run(ctx); err != nil {
		log.Fatalf("app run failed: %v", err)
	}

	// shutdown the app
	_ = app.Shutdown(context.Background())
}
