package queue

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// ---- Size and limit constants ----
const (
	maxStringLen  = 0xFFFF     // 65535
	maxExtrasLen  = 0xFFFF     // 65535
	maxPayloadLen = 0xFFFFFFFF // 4294967295
)

// serializeOpToBB writes an Op into the provided pooled ByteBuffer and
// returns a slice pointing to the payload bytes within the buffer (if any).
func serializeOpToBB(op *QueueOp, bb *bytebufferpool.ByteBuffer) ([]byte, error) {
	if bb == nil {
		return nil, errors.New("nil buffer")
	}
	// reset buffer
	bb.B = bb.B[:0]
	// write version
	bb.WriteByte(opRecordVersion)

	writeString := func(s string) error {
		if len(s) > maxStringLen {
			return io.ErrShortBuffer
		}
		l := uint16(len(s))
		if err := binary.Write(bb, binary.BigEndian, l); err != nil {
			return err
		}
		if l > 0 {
			if _, err := bb.WriteString(s); err != nil {
				return err
			}
		}
		return nil
	}

	if err := writeString(string(op.Handler)); err != nil {
		return nil, err
	}
	if err := writeString(op.Thread); err != nil {
		return nil, err
	}
	if err := writeString(op.ID); err != nil {
		return nil, err
	}
	if err := binary.Write(bb, binary.BigEndian, op.TS); err != nil {
		return nil, err
	}
	if err := binary.Write(bb, binary.BigEndian, op.EnqSeq); err != nil {
		return nil, err
	}

	if op.Extras == nil {
		if err := binary.Write(bb, binary.BigEndian, uint16(0)); err != nil {
			return nil, err
		}
	} else {
		if len(op.Extras) > maxExtrasLen {
			return nil, io.ErrShortBuffer
		}
		if err := binary.Write(bb, binary.BigEndian, uint16(len(op.Extras))); err != nil {
			return nil, err
		}
		for k, v := range op.Extras {
			if err := writeString(k); err != nil {
				return nil, err
			}
			if err := writeString(v); err != nil {
				return nil, err
			}
		}
	}

	// record payload start
	var payloadSlice []byte
	if op.Payload == nil {
		if err := binary.Write(bb, binary.BigEndian, uint32(0)); err != nil {
			return nil, err
		}
	} else {
		if len(op.Payload) > maxPayloadLen {
			return nil, io.ErrShortBuffer
		}
		// write length
		if err := binary.Write(bb, binary.BigEndian, uint32(len(op.Payload))); err != nil {
			return nil, err
		}
		// record slice start
		start := len(bb.B)
		if len(op.Payload) > 0 {
			if _, err := bb.Write(op.Payload); err != nil {
				return nil, err
			}
		}
		payloadSlice = bb.B[start : start+len(op.Payload)]
	}
	return payloadSlice, nil
}

// Pools for reusing QueueOp and QueueItem objects.
var queueOpPool = sync.Pool{New: func() any { return &QueueOp{} }}
var queueItemPool = sync.Pool{New: func() any { return &QueueItem{} }}

// DefaultIngestQueue
var DefaultIngestQueue *IngestQueue

// Item helpers moved from types.go
func (it *QueueItem) Done() {
	it.once.Do(func() {
		if it.Q != nil {
			atomic.AddInt64(&it.Q.inFlight, -1)
			it.Q = nil
		}
		if it.Sb != nil {
			it.Sb.release()
			it.Sb = nil
		} else if it.Buf != nil {
			if cap(it.Buf.B) > maxPooledBuffer {
				it.Buf = nil
			} else {
				bytebufferpool.Put(it.Buf)
			}
		}
	})
}

func (it *QueueItem) CopyPayload() []byte {
	if it == nil {
		return nil
	}
	if it.Sb != nil && it.Sb.bb != nil {
		src := it.Sb.bb.B
		dst := make([]byte, len(src))
		copy(dst, src)
		return dst
	}
	if it.Buf != nil {
		src := it.Buf.B
		dst := make([]byte, len(src))
		copy(dst, src)
		return dst
	}
	if it.Op != nil && it.Op.Payload != nil {
		dst := make([]byte, len(it.Op.Payload))
		copy(dst, it.Op.Payload)
		return dst
	}
	return nil
}

// sharedBuf methods
func (s *SharedBuf) inc() { atomic.AddInt32(&s.refs, 1) }
func (s *SharedBuf) dec() bool {
	if atomic.AddInt32(&s.refs, -1) == 0 {
		return true
	}
	return false
}

// Errs
var ErrQueueFull = errors.New("ingest queue full")
var ErrQueueClosed = errors.New("ingest queue closed")
