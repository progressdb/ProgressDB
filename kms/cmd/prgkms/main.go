package main

import (
	"context"
	"flag"
	"log"

	server "github.com/progressdb/kms/internal"
	"github.com/progressdb/kms/pkg/config"
	security "github.com/progressdb/kms/pkg/core"
	"github.com/progressdb/kms/pkg/embedded"
)

func main() {
	var (
		endpoint = flag.String("endpoint", "127.0.0.1:6820", "HTTP endpoint address (host:port) or full URL")
		dataDir  = flag.String("data-dir", "./core-data", "data directory")
		cfgPath  = flag.String("config", "", "optional config yaml")
		embedded = flag.Bool("embedded", false, "run in embedded mode (no HTTP server)")
	)
	flag.Parse()

	// Load full config if provided, otherwise use defaults
	var cfg *config.Config
	var masterHex string

	if *cfgPath != "" {
		var err error
		masterHex, err = config.LoadMasterKeyFromConfig(*cfgPath)
		if err != nil {
			log.Fatalf("failed to load master key from config %s: %v", *cfgPath, err)
		}

		cfg, err = config.LoadFromFile(*cfgPath)
		if err != nil {
			log.Fatalf("failed to load config from file %s: %v", *cfgPath, err)
		}
	} else {
		cfg = config.DefaultConfig()
	}

	// Override with command line flags if provided
	if *endpoint != "127.0.0.1:6820" {
		cfg.Endpoint = *endpoint
	}
	if *dataDir != "./core-data" {
		cfg.DataDir = *dataDir
	}

	// Validate config
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	// Initialize core provider if master key is available
	var provider security.KMSProvider
	if masterHex != "" {
		p, errProv := security.NewHashicorpProviderFromHex(context.Background(), masterHex)
		if errProv != nil {
			log.Fatalf("failed to init provider: %v", errProv)
		}
		provider = p
	}

	// Choose mode based on flag
	if *embedded {
		runEmbeddedMode(provider, cfg)
	} else {
		runServiceMode(provider, cfg)
	}
}

func runEmbeddedMode(provider security.KMSProvider, cfg *config.Config) {
	embeddedKMS, err := embedded.New(context.Background(), &embedded.Config{
		MasterKey: cfg.GetMasterKey(),
		DataDir:   cfg.DataDir,
	})
	if err != nil {
		log.Fatalf("failed to create embedded KMS: %v", err)
	}
	defer embeddedKMS.Close()

	// Example usage - in real scenarios, this would be used programmatically
	log.Printf("Embedded KMS initialized successfully")
	log.Printf("Provider enabled: %v", embeddedKMS.Enabled())

	if err := embeddedKMS.Health(); err != nil {
		log.Printf("Health check failed: %v", err)
	} else {
		log.Printf("Health check passed")
	}
}

func runServiceMode(provider security.KMSProvider, cfg *config.Config) {
	// Create and start server
	srv, err := server.New(cfg.Endpoint, provider, cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	defer srv.Close()

	if err := srv.Start(cfg.Endpoint); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
