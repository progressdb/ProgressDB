package wally

import (
	"fmt"
	"progressdb/pkg/ingest/types"
	"progressdb/pkg/store/keys"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
)

// Pebble log entry key bounds and markers (inline prefix definitions)
const (
	pebbleLogKeyLowerBound = "00000000000000000000"
	pebbleLogKeyUpperBound = "99999999999999999999"
	pebbleSyncMarkerKey    = "sync:marker"
)

var (
	ErrCorrupt    = fmt.Errorf("wal corrupt")
	ErrClosed     = fmt.Errorf("wal closed")
	ErrNotFound   = fmt.Errorf("not found")
	ErrOutOfOrder = fmt.Errorf("out of order")
	ErrOutOfRange = fmt.Errorf("out of range")
	ErrEmptyLog   = fmt.Errorf("empty log")
)

type Options struct {
	NoSync           bool
	SegmentSize      int
	LogFormat        int
	SegmentCacheSize int
	NoCopy           bool
	AllowEmpty       bool
	DirPerms         int
	FilePerms        int
}

var DefaultOptions = &Options{
	NoSync:           false,
	SegmentSize:      20971520,
	LogFormat:        0,
	SegmentCacheSize: 2,
	NoCopy:           false,
	AllowEmpty:       false,
	DirPerms:         0750,
	FilePerms:        0640,
}

type Log struct {
	mu     sync.RWMutex
	db     *pebble.DB
	path   string
	opts   Options
	closed bool
}

func Open(path string, opts *Options) (*Log, error) {
	if opts == nil {
		opts = DefaultOptions
	}

	var err error
	path, err = abs(path)
	if err != nil {
		return nil, err
	}

	pebbleOpts := &pebble.Options{
		DisableWAL: opts.NoSync,
	}

	db, err := pebble.Open(path, pebbleOpts)
	if err != nil {
		return nil, fmt.Errorf("open pebble wal: %w", err)
	}

	l := &Log{
		path: path,
		opts: *opts,
		db:   db,
	}

	return l, nil
}

func abs(path string) (string, error) {
	if path == ":memory:" {
		return "", fmt.Errorf("in-memory log not supported")
	}
	return path, nil
}

func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		if l.db == nil {
			return ErrClosed
		}
		return nil
	}

	err := l.db.Close()
	l.closed = true
	return err
}

func (l *Log) Write(index uint64, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	key := fmt.Sprintf("%020d", index)
	return l.db.Set([]byte(key), data, l.writeOpts())
}

func (l *Log) WriteWithSequence(data []byte) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return 0, ErrClosed
	}

	nextSeq, err := l.getNextSequence()
	if err != nil {
		return 0, fmt.Errorf("get next sequence: %w", err)
	}

	key := fmt.Sprintf("%020d", nextSeq)
	if err := l.db.Set([]byte(key), data, l.writeOpts()); err != nil {
		return 0, fmt.Errorf("write operation: %w", err)
	}

	return nextSeq, nil
}

func (l *Log) getNextSequence() (uint64, error) {
	data, closer, err := l.db.Get([]byte(keys.WALMetaNextSequenceKey))
	if closer != nil {
		defer closer.Close()
	}

	var nextSeq uint64 = 1
	if err == nil {
		if parsed, err := strconv.ParseUint(string(data), 10, 64); err == nil {
			nextSeq = parsed
		}
	}

	nextSeq++
	batch := l.db.NewBatch()
	defer batch.Close()
	batch.Set([]byte(keys.WALMetaNextSequenceKey), []byte(fmt.Sprintf("%d", nextSeq)), l.writeOpts())

	if err := l.db.Apply(batch, l.writeOpts()); err != nil {
		return 0, fmt.Errorf("persist sequence: %w", err)
	}

	return nextSeq - 1, nil
}

func (l *Log) writeOpts() *pebble.WriteOptions {
	if l.opts.NoSync {
		return &pebble.WriteOptions{Sync: false}
	}
	return &pebble.WriteOptions{Sync: true}
}

func (l *Log) Read(index uint64) ([]byte, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return nil, ErrClosed
	}

	key := fmt.Sprintf("%020d", index)
	data, closer, err := l.db.Get([]byte(key))
	if closer != nil {
		defer closer.Close()
	}

	if err != nil {
		if err == pebble.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("read entry: %w", err)
	}

	return data, nil
}

func (l *Log) FirstIndex() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return 0, ErrClosed
	}

	iter, err := l.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(pebbleLogKeyLowerBound),
		UpperBound: []byte(pebbleLogKeyUpperBound),
	})
	if err != nil {
		return 0, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	if iter.First() {
		key := iter.Key()
		if len(key) == 20 {
			if seq, err := strconv.ParseUint(string(key), 10, 64); err == nil {
				return seq, nil
			}
		}
	}

	return 0, nil
}

func (l *Log) LastIndex() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return 0, ErrClosed
	}

	iter, err := l.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(pebbleLogKeyLowerBound),
		UpperBound: []byte(pebbleLogKeyUpperBound),
	})
	if err != nil {
		return 0, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	var lastSeq uint64
	if iter.Last() {
		for ; iter.Valid(); iter.Prev() {
			key := iter.Key()
			if len(key) == 20 {
				if seq, err := strconv.ParseUint(string(key), 10, 64); err == nil {
					lastSeq = seq
					break
				}
			}
		}
	}

	return lastSeq, nil
}

func (l *Log) TruncateFront(index uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	batch := l.db.NewBatch()
	iter, err := l.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(pebbleLogKeyLowerBound),
		UpperBound: []byte(fmt.Sprintf("%020d", index-1)),
	})
	if err != nil {
		return fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		batch.Delete(iter.Key(), l.writeOpts())
	}

	return l.db.Apply(batch, l.writeOpts())
}

func (l *Log) TruncateSequences(seqs []uint64) error {
	if len(seqs) == 0 {
		return nil
	}

	// Sort sequences for efficient processing
	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	batch := l.db.NewBatch()
	defer batch.Close()

	// Queue all sequence deletions in single batch
	for _, seq := range seqs {
		key := fmt.Sprintf("%020d", seq)
		batch.Delete([]byte(key), l.writeOpts())
	}

	// Single atomic commit with sync
	return l.db.Apply(batch, l.writeOpts())
}

func (l *Log) Sync() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return ErrClosed
	}

	// Pebble doesn't have direct Sync() method, sync is handled via WriteOptions
	// For WAL sync, we write a sync marker
	return l.db.Set([]byte(pebbleSyncMarkerKey), []byte(fmt.Sprintf("%d", time.Now().UnixNano())), &pebble.WriteOptions{Sync: true})
}

func (l *Log) IsEmpty() (bool, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return false, ErrClosed
	}

	first, err := l.FirstIndex()
	if err != nil {
		return false, err
	}

	last, err := l.LastIndex()
	if err != nil {
		return false, err
	}

	return first == 0 && last == 0, nil
}

// Ensure Log implements types.WAL interface
var _ types.WAL = (*Log)(nil)
