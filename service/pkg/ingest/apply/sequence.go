package apply

import (
	"fmt"

	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
)

type MessageSequencer struct {
	kv           *KVManager
	indexManager *IndexManager
}

func NewMessageSequencer(im *IndexManager, kv *KVManager) *MessageSequencer {
	return &MessageSequencer{
		kv:           kv,
		indexManager: im,
	}
}

func (m *MessageSequencer) GetFinalThreadKey(threadKey string) (string, error) {
	if keys.ValidateThreadKey(threadKey) != nil && keys.ValidateThreadPrvKey(threadKey) != nil {
		return "", fmt.Errorf("invalid thread key format: %s - expected t:<threadID>", threadKey)
	}
	return threadKey, nil
}

func (m *MessageSequencer) resolveMessageFinalKeyFromDB(msgKey string) (string, bool) {
	if storedb.Client == nil {
		return "", false
	}
	prefix := msgKey + ":"

	iter, err := storedb.Client.NewIter(nil)
	if err != nil {
		return "", false
	}
	defer iter.Close()

	iter.SeekGE([]byte(prefix))
	if iter.Valid() && len(iter.Key()) > len(prefix) && string(iter.Key()[:len(prefix)]) == prefix {
		return string(iter.Key()), true
	}
	return "", false
}
