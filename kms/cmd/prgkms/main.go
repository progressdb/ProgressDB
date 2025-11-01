package main

import (
	"context"
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
	if err := config.LoadConfig(*configPath); err != nil {
		log.Fatalf("failed to load configuration: %v", err)
	}

	// Load master key
	if err := config.LoadMasterKey(*configPath); err != nil {
		log.Fatalf("failed to load master key: %v", err)
	}

	cfg := config.GetConfig()
	masterKey := config.GetMasterKey()

	// Initialize provider
	provider, err := security.NewHashicorpProviderFromHex(context.Background(), masterKey)
	if err != nil {
		log.Fatalf("failed to init provider: %v", err)
	}

	// Create and start server
	srv, err := server.New(*address, provider, cfg.DataDir)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}
	defer srv.Close()

	log.Printf("Starting KMS server on %s", *address)
	log.Printf("Data directory: %s", cfg.DataDir)

	if err := srv.Start(*address); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
