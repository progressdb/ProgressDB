package queue

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
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

// compressData compresses input bytes using gzip and returns the compressed
// bytes or an error.
func compressData(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decompressData decompresses gzip-compressed bytes and returns the
// decompressed payload.
func decompressData(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// serializeOp encodes an Op into a custom binary format:
// [version][handler][thread][id][ts][enqseq][extras][payload]
func serializeOp(op *QueueOp) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(opRecordVersion)

	writeString := func(s string) error {
		if len(s) > maxStringLen {
			return io.ErrShortBuffer
		}
		l := uint16(len(s))
		if err := binary.Write(&buf, binary.BigEndian, l); err != nil {
			return err
		}
		if l > 0 {
			if _, err := buf.WriteString(s); err != nil {
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
	if err := binary.Write(&buf, binary.BigEndian, op.TS); err != nil {
		return nil, err
	}
	if err := binary.Write(&buf, binary.BigEndian, op.EnqSeq); err != nil {
		return nil, err
	}

	if op.Extras == nil {
		if err := binary.Write(&buf, binary.BigEndian, uint16(0)); err != nil {
			return nil, err
		}
	} else {
		if len(op.Extras) > maxExtrasLen {
			return nil, io.ErrShortBuffer
		}
		if err := binary.Write(&buf, binary.BigEndian, uint16(len(op.Extras))); err != nil {
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

	if op.Payload == nil {
		if err := binary.Write(&buf, binary.BigEndian, uint32(0)); err != nil {
			return nil, err
		}
	} else {
		if len(op.Payload) > maxPayloadLen {
			return nil, io.ErrShortBuffer
		}
		if err := binary.Write(&buf, binary.BigEndian, uint32(len(op.Payload))); err != nil {
			return nil, err
		}
		if len(op.Payload) > 0 {
			if _, err := buf.Write(op.Payload); err != nil {
				return nil, err
			}
		}
	}
	return buf.Bytes(), nil
}

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

// deserializeOp decodes a binary Op previously encoded by serializeOp.
func deserializeOp(b []byte) (*QueueOp, error) {
	if len(b) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	if b[0] != opRecordVersion {
		return nil, fmt.Errorf("unsupported Op record version: 0x%02x", b[0])
	}

	r := bytes.NewReader(b)
	// Skip version
	if _, err := r.ReadByte(); err != nil {
		return nil, err
	}
	readString := func() (string, error) {
		var l uint16
		if err := binary.Read(r, binary.BigEndian, &l); err != nil {
			return "", err
		}
		if l == 0 {
			return "", nil
		}
		bs := make([]byte, l)
		if _, err := io.ReadFull(r, bs); err != nil {
			return "", err
		}
		return string(bs), nil
	}

	handlerStr, err := readString()
	if err != nil {
		return nil, err
	}
	thread, err := readString()
	if err != nil {
		return nil, err
	}
	id, err := readString()
	if err != nil {
		return nil, err
	}
	var ts int64
	if err := binary.Read(r, binary.BigEndian, &ts); err != nil {
		return nil, err
	}
	var enqseq uint64
	if err := binary.Read(r, binary.BigEndian, &enqseq); err != nil {
		return nil, err
	}
	var extrasCount uint16
	if err := binary.Read(r, binary.BigEndian, &extrasCount); err != nil {
		return nil, err
	}
	extras := make(map[string]string, extrasCount)
	for i := 0; i < int(extrasCount); i++ {
		k, err := readString()
		if err != nil {
			return nil, err
		}
		v, err := readString()
		if err != nil {
			return nil, err
		}
		extras[k] = v
	}
	var payloadLen uint32
	if err := binary.Read(r, binary.BigEndian, &payloadLen); err != nil {
		return nil, err
	}
	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}
	}

	op := &QueueOp{
		Handler:   HandlerID(handlerStr),
		Thread:    thread,
		ID:        id,
		Payload:   payload,
		TS:        ts,
		EnqSeq:    enqseq,
		Extras:    extras,
		WalOffset: -1,
	}
	return op, nil
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

// newSharedBuf constructs a SharedBuf with an initial reference count.
func newSharedBuf(bb *bytebufferpool.ByteBuffer, initial int32) *SharedBuf {
	return &SharedBuf{bb: bb, refs: initial}
}
