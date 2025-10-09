package ingest

import (
	"sync/atomic"

	"progressdb/pkg/logger"
)

// walStub is a very small WAL placeholder used by the initial ingest
// processor. It does not write to disk yet — instead it provides a stable
// offset for batches. Replace with a real WAL implementation later.
type walStub struct {
	off uint64
}

func (w *walStub) AppendBatch(_ []byte) uint64 {
	// return incrementing offset
	o := atomic.AddUint64(&w.off, 1)
	logger.Debug("wal_stub_append", "offset", o)
	return o
}

var globalWAL = &walStub{}
