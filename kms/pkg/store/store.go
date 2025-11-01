package store

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	pebble "github.com/cockroachdb/pebble"
)

type Store struct {
	db *pebble.DB
}

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

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) SaveKeyMeta(keyID string, meta []byte) error {
	return s.db.Set([]byte("meta:"+keyID), meta, pebble.Sync)
}

func (s *Store) GetKeyMeta(keyID string) ([]byte, error) {
	v, closer, err := s.db.Get([]byte("meta:" + keyID))
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

func (s *Store) IterateMeta(fn func(key string, meta []byte) error) error {
	it, err := s.db.NewIter(nil)
	if err != nil {
		return err
	}
	defer it.Close()
	prefix := []byte("meta:")
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

func (s *Store) BackupKeyMeta(keyID, backupDir string) error {
	b, err := s.GetKeyMeta(keyID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(backupDir, keyID+"."+time.Now().Format("20060102T150405"))
	return os.WriteFile(path, b, 0600)
}
