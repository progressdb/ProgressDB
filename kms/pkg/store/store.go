package store

import (
	"bytes"
	"os"
	"path/filepath"

	pebble "github.com/cockroachdb/pebble"
)

// Store handles DEK storage and retrieval
type Store struct {
	db *pebble.DB
}

// New creates a new store instance
func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := pebble.Open(path, &pebble.Options{})
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the store
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// SaveKeyMeta stores a wrapped DEK
func (s *Store) SaveKeyMeta(keyID string, wrappedDEK []byte) error {
	storedKey := FormatDEKKey(keyID)
	return s.db.Set([]byte(storedKey), wrappedDEK, pebble.Sync)
}

// GetKeyMeta retrieves a wrapped DEK
func (s *Store) GetKeyMeta(keyID string) ([]byte, error) {
	storedKey := FormatDEKKey(keyID)
	v, closer, err := s.db.Get([]byte(storedKey))
	if err != nil {
		return nil, err
	}
	if closer != nil {
		defer closer.Close()
	}
	// copy value
	out := make([]byte, len(v))
	copy(out, v)
	return out, nil
}

// IterateMeta iterates over all stored DEKs
func (s *Store) IterateMeta(fn func(key string, meta []byte) error) error {
	it, err := s.db.NewIter(nil)
	if err != nil {
		return err
	}
	defer it.Close()
	prefix := []byte(DEKPrefix)
	for ok := it.First(); ok; ok = it.Next() {
		k := it.Key()
		if !bytes.HasPrefix(k, prefix) {
			continue
		}
		v := it.Value()
		// copy bytes
		kb := make([]byte, len(k)-len(prefix))
		copy(kb, k[len(prefix):])
		vb := make([]byte, len(v))
		copy(vb, v)
		if err := fn(string(kb), vb); err != nil {
			return err
		}
	}
	return nil
}
