package wally

import (
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/cockroachdb/pebble"
)

const (
	pebbleLogKeyLowerBound = "00000000000000000000"
	pebbleLogKeyUpperBound = "99999999999999999999"
)

var (
	ErrClosed   = fmt.Errorf("wal closed")
	ErrNotFound = fmt.Errorf("not found")
)

type Options struct {
	NoSync bool
}

var DefaultOptions = &Options{
	NoSync: false,
}

type Log struct {
	mu     sync.RWMutex
	db     *pebble.DB
	path   string
	opts   Options
	closed bool
}

func Open(path string) (*Log, error) {
	pebbleOpts := &pebble.Options{
		DisableWAL: false, // NEVER disable
	}

	db, err := pebble.Open(path, pebbleOpts)
	if err != nil {
		return nil, fmt.Errorf("open pebble wal: %w", err)
	}

	return &Log{
		path: path,
		opts: *DefaultOptions,
		db:   db,
	}, nil
}

func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	l.closed = true
	return l.db.Close()
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

	if err == pebble.ErrNotFound {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read entry: %w", err)
	}

	// Make a copy since Pebble data is only valid until closer is called
	result := make([]byte, len(data))
	copy(result, data)
	return result, nil
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

	if iter.Last() {
		key := iter.Key()
		if len(key) == 20 {
			if seq, err := strconv.ParseUint(string(key), 10, 64); err == nil {
				return seq, nil
			}
		}
	}

	return 0, nil
}

func (l *Log) TruncateFront(index uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	batch := l.db.NewBatch()
	defer batch.Close()

	upperBound := fmt.Sprintf("%020d", index)
	iter, err := l.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte(pebbleLogKeyLowerBound),
		UpperBound: []byte(upperBound),
	})
	if err != nil {
		return fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		if err := batch.Delete(iter.Key(), nil); err != nil {
			return err
		}
	}

	return batch.Commit(l.writeOpts())
}

func (l *Log) TruncateSequences(seqs []uint64) error {
	if len(seqs) == 0 {
		return nil
	}

	sort.Slice(seqs, func(i, j int) bool { return seqs[i] < seqs[j] })

	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	batch := l.db.NewBatch()
	defer batch.Close()

	for _, seq := range seqs {
		key := fmt.Sprintf("%020d", seq)
		if err := batch.Delete([]byte(key), nil); err != nil {
			return err
		}
	}

	return batch.Commit(l.writeOpts())
}

func (l *Log) Sync() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	// Force a sync by writing empty batch with Sync: true
	batch := l.db.NewBatch()
	defer batch.Close()
	return batch.Commit(&pebble.WriteOptions{Sync: true})
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

func (l *Log) writeOpts() *pebble.WriteOptions {
	return &pebble.WriteOptions{
		Sync: !l.opts.NoSync, // Sync unless NoSync is true
	}
}
