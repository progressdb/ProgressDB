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

	// initialize logger after config is fully loaded
	logger.Init(eff.Config.Logging.Level, eff.DBPath)
	defer logger.Sync()

	logger.Info("effective_config_loaded", "source", eff.Source, "addr", eff.Addr, "db_path", eff.DBPath)
	logger.Info("config_validation_passed")

	// set to maximum cpu's available
	numCPU := runtime.NumCPU()
	runtime.GOMAXPROCS(numCPU)
	logger.Info("system_logical_cores", "logical_cores", numCPU)

	// lower worker count to 2 x cpu logical cores
	cc := &eff.Config.Ingest.Compute
	maxAllowedWorkers := numCPU * 2
	if cc.WorkerCount > maxAllowedWorkers {
		logger.Warn("worker_count_capped", "requested", cc.WorkerCount, "capped_to", maxAllowedWorkers)
		cc.WorkerCount = maxAllowedWorkers
	}

	// init database folders and ensure the filesystem layout.
	if err := state.Init(eff.DBPath); err != nil {
		logger.Error("state_dirs_setup_failed", "error", err)
		fmt.Fprintf(os.Stderr, "state_dirs_setup_failed: %v\n", err)
		shutdown.Abort(fmt.Sprintf("failed to ensure state directories under %s", eff.DBPath), err, eff.DBPath)
	}

	// initialize app
	app, err := app.New(version, commit, buildDate)
	if err != nil {
		shutdown.Abort("failed to initialize app", err, eff.DBPath)
	}

	// set up context and signal handling for graceful shutdown
	ctx, cancel := shutdown.SetupSignalHandler(context.Background())
	defer cancel()

	// run the app
	if err := app.Run(ctx); err != nil {
		shutdown.Abort("app run failed", err, eff.DBPath)
	}

	// shutdown the app with a bounded timeout so teardown cannot hang forever
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutdownCancel()
	_ = app.Shutdown(shutdownCtx)
}
