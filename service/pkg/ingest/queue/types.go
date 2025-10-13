package queue

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/valyala/bytebufferpool"
)

// HandlerID identifies which action the queue Op should perform.
type HandlerID string

const (
	HandlerMessageCreate  HandlerID = "message.create"
	HandlerMessageUpdate  HandlerID = "message.update"
	HandlerMessageDelete  HandlerID = "message.delete"
	HandlerReactionAdd    HandlerID = "reaction.add"
	HandlerReactionDelete HandlerID = "reaction.delete"
	HandlerThreadCreate   HandlerID = "thread.create"
	HandlerThreadUpdate   HandlerID = "thread.update"
	HandlerThreadDelete   HandlerID = "thread.delete"
)

// Op describes a queue operation and its metadata.
type Op struct {
	Handler   HandlerID         // Which handler to invoke
	Thread    string            // Thread identifier
	ID        string            // Record identifier
	Payload   []byte            // Payload data, may be nil
	TS        int64             // Timestamp (nanoseconds)
	EnqSeq    uint64            // Sequence assigned at enqueue
	WalOffset int64             // WAL offset, -1 if not set
	Extras    map[string]string // Optional metadata (e.g., user id, role)
}

// Item wraps an Op and manages pooled buffer and queue references.
type Item struct {
	Op   *Op
	buf  *bytebufferpool.ByteBuffer
	once sync.Once
	q    *Queue
}

// Done releases all item, buffer, and op resources back to pools.
func (it *Item) Done() {
	it.once.Do(func() {
		if it.q != nil {
			// Ack WAL offset if set, update in-flight count
			if it.Op != nil && it.Op.WalOffset >= 0 {
				it.q.ack(it.Op.WalOffset)
			}
			atomic.AddInt64(&it.q.inFlight, -1)
			it.q = nil
		}
		if it.buf != nil {
			// Only pool reasonably sized buffers
			if cap(it.buf.B) > maxPooledBuffer {
				it.buf = nil
			} else {
				bytebufferpool.Put(it.buf)
				it.buf = nil
			}
		}
		if it.Op != nil {
			it.Op.Payload = nil
			it.Op.Extras = nil
			opPool.Put(it.Op)
			it.Op = nil
		}
		itemPool.Put(it)
	})
}

// Pools for reusing Op and Item objects.
var opPool = sync.Pool{New: func() any { return &Op{} }}
var itemPool = sync.Pool{New: func() any { return &Item{} }}

// Maximum buffer size to pool, larger ones are dropped instead for GC.
var maxPooledBuffer = 256 * 1024 // 256 KiB

const opRecordVersion = 0x1

// serializeOp encodes an Op into a custom binary format:
// [version][handler][thread][id][ts][enqseq][extras][payload]
func serializeOp(op *Op) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(opRecordVersion)

	writeString := func(s string) error {
		if len(s) > 0xFFFF {
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
		if len(op.Extras) > 0xFFFF {
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
		if len(op.Payload) > 0xFFFFFFFF {
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

// deserializeOp decodes a binary Op previously encoded by serializeOp.
func deserializeOp(b []byte) (*Op, error) {
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

	op := &Op{
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

// ErrQueueFull is returned when queue is at capacity.
var ErrQueueFull = errors.New("ingest queue full")

// ErrQueueClosed is returned when enqueueing after queue is closed.
var ErrQueueClosed = errors.New("ingest queue closed")
