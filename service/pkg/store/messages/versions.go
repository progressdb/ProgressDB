package messages

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"
	"progressdb/pkg/store/db/index"
	"progressdb/pkg/store/encryption"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// returns all versions for a given message in order
func ListMessageVersions(msgID string) ([]string, error) {
	if index.IndexDB == nil {
		return nil, fmt.Errorf("pebble not opened; call Open first")
	}
	prefix := keys.GenAllMessageVersionsPrefix(msgID)
	iter, err := index.IndexDB.NewIter(&pebble.IterOptions{})
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

		// Get thread ID from message to fetch KMS metadata (only once)
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

		// Decrypt using encryption utils (handles both field-level and full message)
		decrypted, err := encryption.DecryptMessageData(kmsMeta, v)
		if err != nil {
			return nil, fmt.Errorf("decryption failed: %w", err)
		}

		out = append(out, string(decrypted))
	}
	return out, iter.Error()
}

// returns latest version for message, error if not found
func GetLatestMessage(msgID string) (string, error) {
	tr := telemetry.Track("storedb.get_latest_message")
	defer tr.Finish()

	tr.Mark("list_versions")
	vers, err := ListMessageVersions(msgID)
	if err != nil {
		return "", err
	}
	if len(vers) == 0 {
		return "", fmt.Errorf("no versions found for message %s", msgID)
	}
	return vers[len(vers)-1], nil
}
