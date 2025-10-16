package sensor

import (
	"log"
	"runtime"
	"time"

	"golang.org/x/sys/unix"
)

// sensor struct
type Sensor struct {
	config MonitorConfig
	stopCh chan struct{}
}

// monitor config
type MonitorConfig struct {
	PollInterval   time.Duration
	DiskHighPct    int
	DiskLowPct     int
	MemHighPct     int
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
	close(s.stopCh)
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
		log.Printf("disk usage high: %.2f%%", usedPct)
	}

	// check memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	memUsedPct := float64(m.HeapInuse) / float64(m.HeapSys) * 100
	if memUsedPct > float64(s.config.MemHighPct) {
		log.Printf("memory usage high: %.2f%%", memUsedPct)
	}
}
