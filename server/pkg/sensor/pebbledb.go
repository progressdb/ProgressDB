package sensor

import (
	"context"
	"time"

	"progressdb/pkg/ingest"
	"progressdb/pkg/logger"
	"progressdb/pkg/store"
)

// MonitorConfig controls thresholds and intervals for the pebble monitor.
type MonitorConfig struct {
	PollInterval time.Duration

	WALHighBytes uint64
	WALLowBytes  uint64

	DiskHighPct int
	DiskLowPct  int

	// hysteresis window to consider recovery
	RecoveryWindow time.Duration
}

// DefaultMonitorConfig returns sensible defaults.
func DefaultMonitorConfig() MonitorConfig {
	return MonitorConfig{
		PollInterval:   500 * time.Millisecond,
		WALHighBytes:   1 << 30, // 1 GiB
		WALLowBytes:    700 << 20,
		DiskHighPct:    80,
		DiskLowPct:     60,
		RecoveryWindow: 5 * time.Second,
	}
}

// StartPebbleMonitor starts a background monitor that watches Pebble
// metrics and adjusts the processor and sensor accordingly. It returns
// a function to stop the monitor.
func StartPebbleMonitor(ctx context.Context, p *ingest.Processor, s *Sensor, cfg MonitorConfig) context.CancelFunc {
	if cfg.PollInterval <= 0 {
		cfg = DefaultMonitorConfig()
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(cfg.PollInterval)
		defer ticker.Stop()
		var state = "normal"
		var lastCritical time.Time
		fsyncTicker := time.NewTicker(100 * time.Millisecond)
		defer fsyncTicker.Stop()
		// capture original processor batch params so we can restore them
		origMax, origFlush := p.GetBatchParams()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m := store.GetPebbleMetrics()
				hw := s.Snapshot()
				diskUtil := 0
				if hw.DiskTotal > 0 {
					used := hw.DiskTotal - hw.DiskFree
					diskUtil = int((used * 100) / hw.DiskTotal)
				}

				if m.WALBytes >= cfg.WALHighBytes || diskUtil >= cfg.DiskHighPct {
					logger.Warn("pebble_monitor: entering paused state", "wal_bytes", m.WALBytes, "disk_util", diskUtil)
					p.Pause()
					s.SendThrottle(ThrottleRequest{Source: "pebble_monitor", Reason: "wal_or_disk_high", Severity: 1.0})
					state = "paused"
					lastCritical = time.Now()
					continue
				}

				if state == "paused" {
					if time.Since(lastCritical) > cfg.RecoveryWindow && m.WALBytes <= cfg.WALLowBytes && diskUtil <= cfg.DiskLowPct {
						logger.Info("pebble_monitor: recovering from paused state")
						p.Resume()
						s.SendThrottle(ThrottleRequest{Source: "pebble_monitor", Reason: "recovered", Severity: 0})
						state = "normal"
					}
					continue
				}

				if m.WALBytes >= cfg.WALLowBytes || diskUtil >= cfg.DiskHighPct {
					logger.Warn("pebble_monitor: degrading batch params", "wal_bytes", m.WALBytes, "disk_util", diskUtil)
					// compute degraded params from current settings
					curMax, curFlush := p.GetBatchParams()
					if curMax > 1 {
						curMax = curMax / 2
					}
					if curFlush < time.Second { // avoid overflow
						curFlush = curFlush * 2
					}
					p.SetBatchParams(curMax, curFlush)
					s.SendThrottle(ThrottleRequest{Source: "pebble_monitor", Reason: "wal_high", Severity: 0.6})
					state = "degraded"
					continue
				}

				if state == "degraded" {
					if m.WALBytes < cfg.WALLowBytes && diskUtil < cfg.DiskLowPct {
						logger.Info("pebble_monitor: restoring batch params")
						p.SetBatchParams(origMax, origFlush)
						state = "normal"
					}
				}
			case <-fsyncTicker.C:
				pw := store.GetPendingWrites()
				if pw > 0 {
					logger.Debug("pebble_monitor: triggering group fsync", "pending_writes", pw)
					if err := store.ForceSync(); err == nil {
						store.ResetPendingWrites()
					}
				}
			}
		}
	}()
	return cancel
}
