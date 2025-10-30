package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"progressdb/internal/app"
	"progressdb/pkg/config"
	"progressdb/pkg/state"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/shutdown"
	"progressdb/pkg/store/migrations"
	"runtime"
	"time"

	"github.com/joho/godotenv"
)

// set build metadata
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// load .env file if present
	_ = godotenv.Load(".env")

	// initialize centralized logger (env defaults) so startup logs are available.
	logger.Init()
	defer logger.Sync()

	// parse config flags
	flags := config.ParseConfigFlags()
	if !flags.Set["db"] {
		if root := state.ArtifactRoot(); root != "" {
			flags.DB = filepath.Join(root, "database")
		}
	}

	// parse config file
	fileCfg, fileExists, err := config.ParseConfigFile(flags)
	if err != nil {
		shutdown.Abort("failed to load config file", err, flags.DB)
	}

	// parse config env variables
	envCfg, envRes := config.ParseConfigEnvs()

	// load effective config
	eff, err := config.LoadEffectiveConfig(flags, fileCfg, fileExists, envCfg, envRes)
	if err != nil {
		shutdown.Abort("failed to build effective config", err, flags.DB)
	}

	// validate config
	if err := config.ValidateConfig(eff); err != nil {
		shutdown.Abort("invalid configuration", err, eff.DBPath)
	}

	// set global config for use throughout the application
	config.SetConfig(eff.Config)

	// Reinitialize logger with config-driven level (overrides env default)
	logger.InitWithLevel(eff.Config.Logging.Level)

	// set to maximum cpu's available
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	logger.Info("system_logical_cores", "logical_cores", numCPU)

	// done
	cc := &eff.Config.Ingest.Compute
	if cc.WorkerCount > numCPU {
		logger.Warn("worker_count_capped", "requested", cc.WorkerCount, "capped_to", numCPU)
		cc.WorkerCount = numCPU
	}

	// init database folders and ensure the filesystem layout.
	if err := state.Init(eff.DBPath); err != nil {
		logger.Error("state_dirs_setup_failed", "error", err)
		fmt.Fprintf(os.Stderr, "state_dirs_setup_failed: %v\n", err)
		shutdown.Abort(fmt.Sprintf("failed to ensure state directories under %s", eff.DBPath), err, eff.DBPath)
	}

	// create audit file for audit logs if not present
	auditPath := state.PathsVar.Audit
	if err := logger.AttachAuditFileSink(auditPath); err != nil {
		logger.Error("attach_audit_sink_failed", "error", err)
		shutdown.Abort(fmt.Sprintf("failed to attach audit sink at %s", auditPath), err, eff.DBPath)
	}

	// initialize app
	app, err := app.New(version, commit, buildDate)
	if err != nil {
		shutdown.Abort("failed to initialize app", err, eff.DBPath)
	}

	// set up context and signal handling for graceful shutdown
	ctx, cancel := shutdown.SetupSignalHandler(context.Background())
	defer cancel()

	// run version checks and migrations - before start app
	if _, err := migrations.Run(ctx, version); err != nil {
		shutdown.Abort("progressor run failed", err, eff.DBPath)
	}

	// run the app
	if err := app.Run(ctx); err != nil {
		shutdown.Abort("app run failed", err, eff.DBPath)
	}

	// shutdown the app with a bounded timeout so teardown cannot hang forever
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	_ = app.Shutdown(shutdownCtx)
}
