package shutdown

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/valyala/fasthttp"

	"progressdb/pkg/ingest"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/logger"
	"progressdb/pkg/sensor"
	"progressdb/pkg/store/db/index"
	storedb "progressdb/pkg/store/db/store"
	kms "progressdb/pkg/store/encryption/kms"
	"progressdb/pkg/telemetry"
)

// ShutdownApp performs graceful shutdown of all app components.
// This consolidates shutdown logic from both app.go and shutdown.go.
func ShutdownApp(ctx context.Context, srvFast *fasthttp.Server, rc *kms.RemoteClient, retentionCancel context.CancelFunc, ingestIngestor *ingest.Ingestor, hwSensor *sensor.Sensor) error {
	log.Printf("shutdown: requested")

	// stop accepting new requests
	if srvFast != nil {
		log.Printf("shutdown: stopping FastHTTP server")
		if err := srvFast.Shutdown(); err != nil {
			log.Printf("shutdown: fasthttp shutdown error: %v", err)
		}
	}

	// close kms client
	if rc != nil {
		log.Printf("shutdown: closing KMS client")
		if err := rc.Close(); err != nil {
			log.Printf("shutdown: kms client close error: %v", err)
		}
	}

	// cancel retention scheduler if running
	if retentionCancel != nil {
		log.Printf("shutdown: stopping retention scheduler")
		retentionCancel()
	}

	// ensure ingest queue drains before closing store and stop ingest processor
	if queue.GlobalIngestQueue != nil {
		queue.GlobalIngestQueue.Close()
	}
	if ingestIngestor != nil {
		log.Printf("shutdown: stopping ingestor")
		ingestIngestor.Stop()
	}

	// stop sensor
	if hwSensor != nil {
		log.Printf("shutdown: stopping sensor")
		hwSensor.Stop()
	}

	// close index
	log.Printf("shutdown: closing index")
	if err := index.Close(); err != nil {
		log.Printf("shutdown: index close error: %v", err)
	}

	// flush close the storage
	log.Printf("shutdown: closing store")
	if err := storedb.Close(); err != nil {
		log.Printf("shutdown: store close error: %v", err)
	}

	// close telemetry
	log.Printf("shutdown: closing telemetry")
	telemetry.Close()

	log.Printf("shutdown: complete")
	return nil
}

// SetupSignalHandler installs handlers for SIGINT/SIGTERM and SIGPIPE and
// returns a cancellable context. The returned context is cancelled when any
// of the watched signals arrives. Use the cancel function to stop watching
// and to release resources.
func SetupSignalHandler(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)

	// handle interrupt/terminate for graceful shutdown
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sigc
		logger.Info("signal_received", "signal", s.String(), "msg", "shutdown requested")
		cancel()
	}()

	// watch for SIGPIPE and dump goroutine stacks to aid diagnostics
	sigpipe := make(chan os.Signal, 1)
	signal.Notify(sigpipe, syscall.SIGPIPE)
	go func() {
		s := <-sigpipe
		logger.Info("signal_received", "signal", s.String(), "msg", "SIGPIPE - dumping goroutine stacks")
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		logger.Info("goroutine_stack_dump", "dump", string(buf[:n]))
		cancel()
	}()

	return ctx, cancel
}
