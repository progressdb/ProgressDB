package queue

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/cockroachdb/pebble"
)

var (
	ErrCorrupt    = fmt.Errorf("wal corrupt")
	ErrClosed     = fmt.Errorf("wal closed")
	ErrNotFound   = fmt.Errorf("not found")
	ErrOutOfOrder = fmt.Errorf("out of order")
	ErrOutOfRange = fmt.Errorf("out of range")
	ErrEmptyLog   = fmt.Errorf("empty log")
)

// Options for WAL Log behavior.
type Options struct {
	// NoSync disables fsync after writes (unsafe, less durable).
	NoSync bool
	// SegmentSize target segment size (default 20 MB; kept for compatibility).
	SegmentSize int
	// LogFormat format for log files (compatibility).
	LogFormat int
	// SegmentCacheSize max segments cached in memory (compatibility).
	SegmentCacheSize int
	// NoCopy returns raw data slice in Read (compatibility).
	NoCopy bool
	// AllowEmpty permits truncating all entries (compatibility).
	AllowEmpty bool
	// DirPerms and FilePerms set directory/file permissions (compatibility).
	DirPerms  int
	FilePerms int
}

// DefaultOptions for Open().
var DefaultOptions = &Options{
	NoSync:           false,    // Fsync after every write
	SegmentSize:      20971520, // 20 MB log segment files (kept for compatibility)
	LogFormat:        0,        // Binary format (kept for compatibility)
	SegmentCacheSize: 2,        // Number of cached in-memory segments (kept for compatibility)
	NoCopy:           false,    // Make a new copy of data for every read call (kept for compatibility)
	AllowEmpty:       false,    // Do not allow empty log. 1+ entries required (kept for compatibility)
	DirPerms:         0750,     // Permissions for created directories (kept for compatibility)
	FilePerms:        0640,     // Permissions for created data files (kept for compatibility)
}

// WALLog represents a write ahead log using Pebble
type Log struct {
	mu     sync.RWMutex
	db     *pebble.DB
	path   string
	opts   Options
	closed bool
}

// Open a new write ahead log using Pebble
func Open(path string, opts *Options) (*Log, error) {
	if opts == nil {
		opts = DefaultOptions
	}

	var err error
	path, err = abs(path)
	if err != nil {
		return nil, err
	}

	// Open Pebble DB
	pebbleOpts := &pebble.Options{
		DisableWAL: opts.NoSync, // Use NoSync to control WAL
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
	// For simplicity, just return the path as-is
	// In a real implementation, you'd use filepath.Abs
	return path, nil
}

// Close the log.
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

// Write an entry to log using provided index (for compatibility)
func (l *Log) Write(index uint64, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	key := fmt.Sprintf("%020d", index)
	return l.db.Set([]byte(key), data, l.writeOpts())
}

// WriteWithSequence writes data and returns the assigned sequence number.
// Used when WAL is enabled to provide persistent sequence generation.
func (l *Log) WriteWithSequence(data []byte) (uint64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return 0, ErrClosed
	}

	// Get next sequence from metadata
	nextSeq, err := l.getNextSequence()
	if err != nil {
		return 0, fmt.Errorf("get next sequence: %w", err)
	}

	// Write the operation data
	key := fmt.Sprintf("%020d", nextSeq)
	if err := l.db.Set([]byte(key), data, l.writeOpts()); err != nil {
		return 0, fmt.Errorf("write operation: %w", err)
	}

	return nextSeq, nil
}

// getNextSequence gets and increments the next sequence number
func (l *Log) getNextSequence() (uint64, error) {
	// Read current sequence
	data, closer, err := l.db.Get([]byte("meta:next_sequence"))
	if closer != nil {
		defer closer.Close()
	}

	var nextSeq uint64 = 1
	if err == nil {
		if parsed, err := strconv.ParseUint(string(data), 10, 64); err == nil {
			nextSeq = parsed
		}
	}

	// Increment and persist
	nextSeq++
	batch := l.db.NewBatch()
	defer batch.Close()
	batch.Set([]byte("meta:next_sequence"), []byte(fmt.Sprintf("%d", nextSeq)), l.writeOpts())

	if err := l.db.Apply(batch, l.writeOpts()); err != nil {
		return 0, fmt.Errorf("persist sequence: %w", err)
	}

	return nextSeq - 1, nil // Return the sequence for this operation
}

// writeOpts returns write options based on NoSync setting
func (l *Log) writeOpts() *pebble.WriteOptions {
	if l.opts.NoSync {
		return &pebble.WriteOptions{Sync: false}
	}
	return &pebble.WriteOptions{Sync: true}
}

// Read an entry from log by index.
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

// FirstIndex returns the index of the first entry in the log.
func (l *Log) FirstIndex() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return 0, ErrClosed
	}

	// Scan for the first entry
	iter, err := l.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("00000000000000000000"), // Minimum possible key
		UpperBound: []byte("99999999999999999999"), // Maximum possible key
	})
	if err != nil {
		return 0, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	if iter.First() {
		key := iter.Key()
		if len(key) == 20 { // Ensure it's an operation key
			if seq, err := strconv.ParseUint(string(key), 10, 64); err == nil {
				return seq, nil
			}
		}
	}

	return 0, nil // Empty log
}

// LastIndex returns the index of the last entry in the log.
func (l *Log) LastIndex() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return 0, ErrClosed
	}

	// Scan for the last entry efficiently by starting from the end
	iter, err := l.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("00000000000000000000"), // Minimum possible key
		UpperBound: []byte("99999999999999999999"), // Maximum possible key
	})
	if err != nil {
		return 0, fmt.Errorf("create iterator: %w", err)
	}
	defer iter.Close()

	var lastSeq uint64
	// Start from the last key and work backwards for efficiency
	if iter.Last() {
		for ; iter.Valid(); iter.Prev() {
			key := iter.Key()
			if len(key) == 20 { // Ensure it's an operation key
				if seq, err := strconv.ParseUint(string(key), 10, 64); err == nil {
					lastSeq = seq
					break // Found the highest sequence, no need to continue
				}
			}
		}
	}

	return lastSeq, nil
}

// TruncateFront removes all entries before the provided index.
func (l *Log) TruncateFront(index uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrClosed
	}

	batch := l.db.NewBatch()
	iter, err := l.db.NewIter(&pebble.IterOptions{
		LowerBound: []byte("00000000000000000000"),
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

// Sync performs an fsync on the log.
func (l *Log) Sync() error {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.closed {
		return ErrClosed
	}

	// Pebble doesn't expose a direct Sync method, but we can force it
	// by writing a marker key with sync enabled
	return l.db.Set([]byte("sync:marker"), []byte(time.Now().Format(time.RFC3339Nano)), pebble.Sync)
}

// IsEmpty returns true if there are no entries in the log.
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
