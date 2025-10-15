package app

import (
	"context"
	"log"
	"time"

	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/store"
)

// Shutdown attempts to gracefully stop all running components.
func (a *App) Shutdown(ctx context.Context) error {
	log.Printf("shutdown: requested")
	a.state = "shutting_down"

	// 1) stop accepting new requests
	if a.srv != nil {
		log.Printf("shutdown: stopping HTTP server")
		ctx2, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := a.srv.Shutdown(ctx2); err != nil {
			log.Printf("shutdown: http shutdown error: %v", err)
		} else {
			log.Printf("shutdown: http shutdown complete")
		}
	}

	// close kms clinet
	if a.rc != nil {
		log.Printf("shutdown: closing KMS client")
		if err := a.rc.Close(); err != nil {
			log.Printf("shutdown: kms client close error: %v", err)
		}
	}

	// cancel retention scheduler if running
	if a.retentionCancel != nil {
		log.Printf("shutdown: stopping retention scheduler")
		a.retentionCancel()
	}

	// ensure ingest queue drains before closing store and stop ingest processor
	if queue.DefaultIngestQueue != nil {
		queue.DefaultIngestQueue.Close()
	}
	if a.ingestProc != nil {
		log.Printf("shutdown: stopping ingest processor")
		a.ingestProc.Stop(ctx)
	}

	// flush close the storage
	log.Printf("shutdown: closing store")
	if err := store.Close(); err != nil {
		log.Printf("shutdown: store close error: %v", err)
	}

	a.state = "stopped"
	log.Printf("shutdown: complete")
	return nil
}
