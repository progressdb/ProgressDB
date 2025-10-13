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

// HandlerID identifies the concrete handler the processor should invoke for
// this Op. This is set by the enqueueing code (API layer) which has the
// authoritative intent for the operation. Processor will use Handler when
// present and will not probe payloads to determine dispatch.
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

// Op is a lightweight in-memory representation of a create/update/delete
// operation destined for the persistence pipeline. Payload may be backed by
// a pooled ByteBuffer; consumers must call Item.Done() when finished.
type Op struct {
	// Handler is an explicit dispatch key set by enqueueing code.
	// Processor MUST call the registered handler matching this value.
	Handler HandlerID
	Thread  string
	ID      string
	// Payload holds the raw bytes for the operation (may be nil).
	Payload []byte
	// TS is an optional client/server timestamp (nanoseconds).
	TS int64
	// EnqSeq is a monotonic enqueue sequence assigned when the op is
	// accepted into the in-memory queue. It is used for deterministic
	// ordering inside batches.
	EnqSeq uint64
	// WalOffset is the durable WAL sequence/offset assigned when this Op
	// was persisted to the WAL. A value of -1 means the Op is not durably
	// stored.
	WalOffset int64
	// Extras holds small metadata extracted from HTTP request headers
	// (e.g. role, identity, request id). It is optional.
	Extras map[string]string
}

// Item wraps an Op and owns a pooled ByteBuffer if one was used. Consumers
// MUST call Done() exactly once after processing the item to return
// pooled resources.
type Item struct {
	Op *Op

	// internal fields
	buf  *bytebufferpool.ByteBuffer
	once sync.Once
	q    *Queue
}

// Done releases internal pooled resources (buffer + op) back to the pool.
func (it *Item) Done() {
	it.once.Do(func() {
		if it.q != nil {
			// Acknowledge WAL entry before releasing queue reference so
			// truncation bookkeeping can run.
			if it.Op != nil && it.Op.WalOffset >= 0 {
				it.q.ack(it.Op.WalOffset)
			}
			atomic.AddInt64(&it.q.inFlight, -1)
			it.q = nil
		}
		if it.buf != nil {
			// avoid retaining huge buffers in the pool
			if cap(it.buf.B) > maxPooledBuffer {
				// drop the buffer so GC can reclaim the underlying array
				it.buf = nil
			} else {
				bytebufferpool.Put(it.buf)
				it.buf = nil
			}
		}
		// clear slice header to avoid retention
		if it.Op != nil {
			it.Op.Payload = nil
			it.Op.Extras = nil
			opPool.Put(it.Op)
			it.Op = nil
		}
		// return Item to pool
		itemPool.Put(it)
	})
}

var opPool = sync.Pool{New: func() any { return &Op{} }}
var itemPool = sync.Pool{New: func() any { return &Item{} }}

// maxPooledBuffer controls the largest buffer size that will be returned
// to the pooled ByteBuffer. Buffers larger than this will be dropped to
// avoid unbounded resident memory.
var maxPooledBuffer = 256 * 1024 // 256 KiB

const opRecordVersion = 0x1

// Custom binary framing (versioned) for Op serialization.
// Format (big-endian):
// 1 byte version (0x1)
// HandlerLen uint16 + Handler bytes
// ThreadLen uint16 + Thread bytes
// IDLen uint16 + ID bytes
// TS int64
// EnqSeq uint64
// ExtrasCount uint16; for each: KeyLen uint16 + Key, ValLen uint16 + Val
// PayloadLen uint32 + Payload bytes

func serializeOp(op *Op) ([]byte, error) {
	var buf bytes.Buffer
	// version
	buf.WriteByte(opRecordVersion)
	// helper
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

	// extras
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

	// payload
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

func deserializeOp(b []byte) (*Op, error) {
	if len(b) == 0 {
		return nil, io.ErrUnexpectedEOF
	}
	// Only support the custom framing format.
	if b[0] != opRecordVersion {
		return nil, fmt.Errorf("unsupported Op record version: 0x%02x", b[0])
	}

	r := bytes.NewReader(b)
	// consume version
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

// ErrQueueFull is returned by TryEnqueue when the queue is at capacity.
// Placed here so callers can reference it from the package.
var ErrQueueFull = errors.New("ingest queue full")

// ErrQueueClosed is returned when enqueue operations are attempted after the queue has closed.
var ErrQueueClosed = errors.New("ingest queue closed")
