package app

import (
	"context"

	"progressdb/pkg/shutdown"
)

// Shutdown attempts to gracefully stop all running components.
func (a *App) Shutdown(ctx context.Context) error {
	a.state = "shutting_down"
	err := shutdown.ShutdownApp(ctx, a.srv, a.srvFast, a.rc, a.retentionCancel, a.ingestIngestor, a.hwSensor)
	if err == nil {
		a.state = "stopped"
	}
	return err
}
