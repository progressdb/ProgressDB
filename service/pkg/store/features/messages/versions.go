package messages

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"
	"progressdb/pkg/store/db/indexdb"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

func ListMessageVersions(messageKey string) ([]string, error) {
	if indexdb.Client == nil {
		return nil, fmt.Errorf("pebble not opened; call Open first")
	}
	prefix, err := keys.GenAllMessageVersionsPrefix(messageKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate versions prefix: %w", err)
	}
	iter, err := indexdb.Client.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var out []string
	var kmsMeta *models.KMSMeta

	for iter.SeekGE([]byte(prefix)); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), []byte(prefix)) {
			break
		}

		v := append([]byte(nil), iter.Value()...)

		if kmsMeta == nil {
			var msg models.Message
			if err := json.Unmarshal(v, &msg); err != nil {
				return nil, fmt.Errorf("invalid message JSON: %w", err)
			}
			if msg.Thread == "" {
				return nil, fmt.Errorf("cannot determine thread for message version")
			}

			kmsMeta, err = encryption.GetThreadKMS(msg.Thread)
			if err != nil {
				return nil, fmt.Errorf("failed to get thread KMS: %w", err)
			}
		}

		decrypted, err := encryption.DecryptMessageData(kmsMeta, v)
		if err != nil {
			return nil, fmt.Errorf("decryption failed: %w", err)
		}

		out = append(out, string(decrypted))
	}
	return out, iter.Error()
}
