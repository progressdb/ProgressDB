package sensor

import (
	"log"
	"runtime"
	"sync"
	"time"

	"golang.org/x/sys/unix"
	"progressdb/pkg/timeutil"
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
		log.Printf("failed to get disk stat: %v", err)
		return
	}
	available := stat.Bavail * uint64(stat.Bsize)
	total := stat.Blocks * uint64(stat.Bsize)
	usedPct := float64(total-available) / float64(total) * 100

	if usedPct > float64(s.config.DiskHighPct) {
		if !s.diskAlert {
			log.Printf("disk usage high: %.2f%% (threshold: %d%%)", usedPct, s.config.DiskHighPct)
			s.diskAlert = true
			s.lastDiskAlert = now
		}
	} else if usedPct < float64(s.config.DiskLowPct) && s.diskAlert {
		// Check if we've been below threshold for the recovery window
		if now.Sub(s.lastDiskAlert) >= s.config.RecoveryWindow {
			log.Printf("disk usage recovered: %.2f%% (below %d%% for %v)", usedPct, s.config.DiskLowPct, s.config.RecoveryWindow)
			s.diskAlert = false
		}
	}

	// check memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsedPct := float64(m.HeapInuse) / float64(m.HeapSys) * 100

	if memUsedPct > float64(s.config.MemHighPct) {
		if !s.memAlert {
			log.Printf("memory usage high: %.2f%% (threshold: %d%%)", memUsedPct, s.config.MemHighPct)
			s.memAlert = true
			s.lastMemAlert = now
		}
	} else if s.memAlert {
		// Memory recovery - check if we've been below threshold for the recovery window
		if now.Sub(s.lastMemAlert) >= s.config.RecoveryWindow {
			log.Printf("memory usage recovered: %.2f%% (below %d%% for %v)", memUsedPct, s.config.MemHighPct, s.config.RecoveryWindow)
			s.memAlert = false
		}
	}
}
