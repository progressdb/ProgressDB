package ingest

import (
	"progressdb/pkg/logger"
)

// Monitor integration point for hardware-aware batching. For now this is a
// stub that can be extended to call into server/pkg/hardware sensor and
// adjust processor batch sizes dynamically.
func AdjustForHardware() {
	logger.Debug("ingest_monitor_stub")
}
