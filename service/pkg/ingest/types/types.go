package types

import (
	"sync"
	"sync/atomic"
)

type HandlerID string

const (
	HandlerMessageCreate HandlerID = "message.create"
	HandlerMessageUpdate HandlerID = "message.update"
	HandlerMessageDelete HandlerID = "message.delete"
	HandlerThreadCreate  HandlerID = "thread.create"
	HandlerThreadUpdate  HandlerID = "thread.update"
	HandlerThreadDelete  HandlerID = "thread.delete"
)

type RequestMetadata struct {
	Role   string `json:"role"`
	UserID string `json:"user_id"`
	ReqID  string `json:"reqid"`
	Remote string `json:"remote"`
}

type QueueOp struct {
	Handler HandlerID
	Payload interface{}
	TS      int64
	EnqSeq  uint64
	Extras  RequestMetadata
}

type WAL interface {
	Write(index uint64, data []byte) error
	Read(index uint64) (data []byte, err error)
	FirstIndex() (index uint64, err error)
	LastIndex() (index uint64, err error)
	TruncateSequences(seqs []uint64) error
	Sync() error
	IsEmpty() (bool, error)
	Close() error
}

type WALRecord struct {
	Offset int64
	Data   []byte
}

type QueueItem struct {
	Op   *QueueOp
	Sb   *SharedBuf
	once sync.Once
	Q    interface{}
}

func (it *QueueItem) JobDone() {
	it.once.Do(func() {
		if it.Sb != nil {
			it.Sb.release()
			it.Sb = nil
		}
	})
}

func (it *QueueItem) SetQueue(q interface{}) {
	it.Q = q
}

func (it *QueueItem) GetQueue() interface{} {
	return it.Q
}

func (it *QueueItem) DoOnce(fn func()) {
	it.once.Do(fn)
}

func (it *QueueItem) ReleaseSharedBuf() {
	if it.Sb != nil {
		it.Sb.release()
		it.Sb = nil
	}
}

type SharedBuf struct {
	data []byte
	refs int32
}

func (sb *SharedBuf) release() {
	atomic.AddInt32(&sb.refs, -1)
}

type BatchEntry struct {
	*QueueOp
	Enq uint64
}
