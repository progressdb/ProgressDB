package sensor

import (
	"runtime"
	"sync"
	"time"

	"progressdb/pkg/config"
	"progressdb/pkg/state/logger"
	"progressdb/pkg/timeutil"

	"golang.org/x/sys/unix"
)

// sensor struct
type Sensor struct {
	config        MonitorConfig
	stopCh        chan struct{}
	stopOnce      sync.Once
	mu            sync.Mutex
	diskAlert     bool
	memAlert      bool
	cpuAlert      bool
	lastDiskAlert time.Time
	lastMemAlert  time.Time
	lastCPUAlert  time.Time
}

// monitor config
type MonitorConfig struct {
	PollInterval   time.Duration
	DiskHighPct    int
	DiskLowPct     int
	MemHighPct     int
	CPUHighPct     int
	RecoveryWindow time.Duration
}

// new sensor
func NewSensor(config MonitorConfig) *Sensor {
	return &Sensor{
		config: config,
		stopCh: make(chan struct{}),
	}
}

// new sensor from config
func NewSensorFromConfig() *Sensor {
	cfg := config.GetConfig()
	monitorConfig := cfg.Sensor.Monitor
	return NewSensor(MonitorConfig{
		PollInterval:   monitorConfig.PollInterval.Duration(),
		DiskHighPct:    monitorConfig.DiskHighPct,
		DiskLowPct:     monitorConfig.DiskLowPct,
		MemHighPct:     monitorConfig.MemHighPct,
		CPUHighPct:     monitorConfig.CPUHighPct,
		RecoveryWindow: monitorConfig.RecoveryWindow.Duration(),
	})
}

// start sensor
func (s *Sensor) Start() {
	go s.run()
}

// stop sensor
func (s *Sensor) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

// run loop
func (s *Sensor) run() {
	ticker := time.NewTicker(s.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.checkHardware()
		case <-s.stopCh:
			return
		}
	}
}

// check hardware
func (s *Sensor) checkHardware() {
	now := timeutil.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	// check storage
	var stat unix.Statfs_t
	err := unix.Statfs("/", &stat)
	if err != nil {
		logger.Error("failed to get disk stat", "error", err)
		return
	}
	available := stat.Bavail * uint64(stat.Bsize)
	total := stat.Blocks * uint64(stat.Bsize)
	usedPct := float64(total-available) / float64(total) * 100

	if usedPct > float64(s.config.DiskHighPct) {
		if !s.diskAlert {
			logger.Warn("disk usage high", "usage_pct", usedPct, "threshold", s.config.DiskHighPct)
			s.diskAlert = true
			s.lastDiskAlert = now
		}
	} else if usedPct < float64(s.config.DiskLowPct) && s.diskAlert {
		// Check if we've been below threshold for the recovery window
		if now.Sub(s.lastDiskAlert) >= s.config.RecoveryWindow {
			logger.Info("disk usage recovered", "usage_pct", usedPct, "threshold", s.config.DiskLowPct, "recovery_window", s.config.RecoveryWindow)
			s.diskAlert = false
		}
	}

	// check memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsedPct := float64(m.HeapInuse) / float64(m.HeapSys) * 100

	if memUsedPct > float64(s.config.MemHighPct) {
		if !s.memAlert {
			logger.Warn("memory usage high", "usage_pct", memUsedPct, "threshold", s.config.MemHighPct)
			s.memAlert = true
			s.lastMemAlert = now
		}
	} else if s.memAlert {
		// Memory recovery - check if we've been below threshold for the recovery window
		if now.Sub(s.lastMemAlert) >= s.config.RecoveryWindow {
			logger.Info("memory usage recovered", "usage_pct", memUsedPct, "threshold", s.config.MemHighPct, "recovery_window", s.config.RecoveryWindow)
			s.memAlert = false
		}
	}
}
