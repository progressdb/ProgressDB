package apply

import (
	"sync"

	"progressdb/pkg/state/logger"
	indexdb "progressdb/pkg/store/db/indexdb"
	storedb "progressdb/pkg/store/db/storedb"

	"github.com/cockroachdb/pebble"
)

type KVManager struct {
	mu      sync.RWMutex
	storeKV map[string][]byte
	indexKV map[string][]byte
	stateKV map[string]string
}

func NewKVManager() *KVManager {
	return &KVManager{
		storeKV: make(map[string][]byte),
		indexKV: make(map[string][]byte),
		stateKV: make(map[string]string),
	}
}

func (kvm *KVManager) SetStoreKV(key string, value []byte) {
	logger.Debug("[KVManager] SetStoreKV", "key", key)
	kvm.mu.Lock()
	defer kvm.mu.Unlock()
	kvm.storeKV[key] = value
}

func (kvm *KVManager) SetIndexKV(key string, value []byte) {
	logger.Debug("[KVManager] SetIndexKV", "key", key)
	kvm.mu.Lock()
	defer kvm.mu.Unlock()
	kvm.indexKV[key] = value
}

func (kvm *KVManager) GetStoreKV(key string) ([]byte, bool) {
	kvm.mu.RLock()
	defer kvm.mu.RUnlock()
	val, ok := kvm.storeKV[key]
	if ok {
		return val, true
	}
	return nil, false
}

func (kvm *KVManager) GetIndexKV(key string) ([]byte, bool) {
	kvm.mu.RLock()
	defer kvm.mu.RUnlock()
	val, ok := kvm.indexKV[key]
	if ok {
		return val, true
	}
	return nil, false
}

func (kvm *KVManager) SetStateKV(key string, value string) {
	logger.Debug("[KVManager] SetStateKV", "key", key)
	kvm.mu.Lock()
	defer kvm.mu.Unlock()
	kvm.stateKV[key] = value
}

func (kvm *KVManager) GetStateKV(key string) (string, bool) {
	kvm.mu.RLock()
	defer kvm.mu.RUnlock()
	val, ok := kvm.stateKV[key]
	return val, ok
}

func (kvm *KVManager) Flush() error {
	kvm.mu.Lock()
	defer kvm.mu.Unlock()

	// Flush store KV
	if len(kvm.storeKV) > 0 {
		storeBatch := storedb.Client.NewBatch()
		defer storeBatch.Close()

		for key, value := range kvm.storeKV {
			logger.Debug("[KVManager] Writing storeKV", "key", key, "len", len(value))
			if err := storeBatch.Set([]byte(key), value, nil); err != nil {
				return err
			}
		}

		// fsync the batch
		if err := storeBatch.Commit(pebble.Sync); err != nil {
			return err
		}
	}

	// Flush index KV
	if len(kvm.indexKV) > 0 {
		indexBatch := indexdb.Client.NewBatch()
		defer indexBatch.Close()

		for key, value := range kvm.indexKV {
			logger.Debug("[KVManager] Writing indexKV", "key", key, "len", len(value))
			if err := indexBatch.Set([]byte(key), value, nil); err != nil {
				return err
			}
		}

		// fsync the batch
		if err := indexBatch.Commit(pebble.Sync); err != nil {
			return err
		}
	}

	// Clear after successful commits
	kvm.storeKV = make(map[string][]byte)
	kvm.indexKV = make(map[string][]byte)
	return nil
}

func (kvm *KVManager) Reset() {
	kvm.mu.Lock()
	defer kvm.mu.Unlock()
	kvm.storeKV = make(map[string][]byte)
	kvm.indexKV = make(map[string][]byte)
	kvm.stateKV = make(map[string]string)
}
