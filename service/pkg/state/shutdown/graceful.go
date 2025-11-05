package shutdown

import (
	"context"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"progressdb/pkg/ingest"
	"progressdb/pkg/ingest/queue"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/state/sensor"
	"progressdb/pkg/state/telemetry"
	"progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"

	"github.com/valyala/fasthttp"
)

// ShutdownApp performs graceful shutdown of all app components.
// This consolidates shutdown logic from both app.go and shutdown.go.
func ShutdownApp(ctx context.Context, srvFast *fasthttp.Server, retentionCancel context.CancelFunc, ingestIngestor *ingest.Ingestor, hwSensor *sensor.Sensor) error {
	logger.Info("shutdown: requested")

	// stop accepting new requests
	if srvFast != nil {
		logger.Info("shutdown: stopping FastHTTP server")
		if err := srvFast.Shutdown(); err != nil {
			logger.Error("shutdown: fasthttp shutdown error", "error", err)
		}
	}

	// cancel retention scheduler if running
	if retentionCancel != nil {
		logger.Info("shutdown: stopping retention scheduler")
		retentionCancel()
	}

	// ensure ingest queue drains before closing store and stop ingest processor
	if queue.GlobalIngestQueue != nil {
		queue.GlobalIngestQueue.Close()
	}
	if ingestIngestor != nil {
		logger.Info("shutdown: stopping ingestor")
		ingestIngestor.Stop()
	}

	// stop sensor
	if hwSensor != nil {
		logger.Info("shutdown: stopping sensor")
		hwSensor.Stop()
	}

	// force sync index to disc before closing
	logger.Info("shutdown: syncing index to disc")
	if err := storedb.Client.Flush(); err != nil {
		logger.Error("shutdown: index force sync error", "error", err)
	}

	// close index
	logger.Info("shutdown: closing index")
	if err := indexdb.Close(); err != nil {
		logger.Error("shutdown: index close error", "error", err)
	}

	// force sync storage to disc before closing
	logger.Info("shutdown: syncing storage to disc")
	if err := storedb.Client.Flush(); err != nil {
		logger.Error("shutdown: store force sync error", "error", err)
	}

	// flush close the storage
	logger.Info("shutdown: closing store")
	if err := storedb.Close(); err != nil {
		logger.Error("shutdown: store close error", "error", err)
	}

	// close telemetry
	logger.Info("shutdown: closing telemetry")
	telemetry.Close()

	logger.Info("shutdown: complete")
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
