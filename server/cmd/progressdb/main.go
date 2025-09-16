package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"progressdb/internal/app"
	"progressdb/pkg/config"
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
		<-sigc
		cancel()
	}()

	// run the app
	if err := app.Run(ctx); err != nil {
		log.Fatalf("app run failed: %v", err)
	}

	// shutdown the app
	_ = app.Shutdown(context.Background())
}
