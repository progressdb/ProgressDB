package main

import (
	"context"
	"flag"
	"log"

	httpserver "github.com/progressdb/kms/internal"
	"github.com/progressdb/kms/pkg/config"
	"github.com/progressdb/kms/pkg/kms"
	"github.com/progressdb/kms/pkg/store"
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

	// Load master key
	masterKey, err := config.LoadMasterKey(cfg)
	if err != nil {
		log.Fatalf("failed to load master key: %v", err)
	}

	// Initialize store
	st, err := store.New(cfg.KMS.DataDir + "/kms.db")
	if err != nil {
		log.Fatalf("failed to create store: %v", err)
	}
	defer st.Close()

	// Initialize KMS
	kmsInstance, err := kms.New(context.Background(), st, masterKey)
	if err != nil {
		log.Fatalf("failed to create KMS: %v", err)
	}

	// Create and start HTTP server
	server := httpserver.NewServer(kmsInstance, *address)

	log.Printf("Starting KMS server on %s", *address)
	log.Printf("Data directory: %s", cfg.KMS.DataDir)

	if err := server.Start(); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
