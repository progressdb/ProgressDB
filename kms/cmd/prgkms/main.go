package main

import (
	"context"
	"encoding/hex"
	"flag"
	"log"

	server "github.com/progressdb/kms/internal"
	"github.com/progressdb/kms/pkg/config"
	security "github.com/progressdb/kms/pkg/core"
)

func main() {
	var (
		configPath = flag.String("config", "", "Path to config file")
		address    = flag.String("addr", "127.0.0.1:6820", "Server address")
	)
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	// Store config globally for other packages
	config.SetGlobalConfig(cfg)

	// Load master key
	masterKey, err := config.LoadMasterKey(cfg)
	if err != nil {
		log.Fatalf("failed to load master key: %v", err)
	}

	// Store master key globally
	config.SetMasterKey(masterKey)

	// Initialize provider
	masterKeyHex := hex.EncodeToString(masterKey)
	provider, err := security.NewHashicorpProviderFromHex(context.Background(), masterKeyHex)
	if err != nil {
		log.Fatalf("failed to init provider: %v", err)
	}

	// Create and start server
	srv, err := server.New(*address, provider, cfg.Encryption.KMS.DataDir)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	defer srv.Close()

	log.Printf("Starting KMS server on %s", *address)
	log.Printf("Data directory: %s", cfg.Encryption.KMS.DataDir)

	if err := srv.Start(*address); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
