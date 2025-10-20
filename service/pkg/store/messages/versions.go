package store

import (
	"bytes"
	"encoding/json"
	"fmt"

	"progressdb/pkg/kms"
	"progressdb/pkg/logger"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	"progressdb/pkg/telemetry"

	"github.com/cockroachdb/pebble"
)

// returns all versions for a given message in order
func ListMessageVersions(msgID string) ([]string, error) {
	if db == nil {
		return nil, fmt.Errorf("index pebble not opened; call index.Open first")
	}
	prefix := []byte("version:msg:" + msgID + ":")
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	var out []string
	var threadKeyID string
	var threadChecked bool
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		v := append([]byte(nil), iter.Value()...)
		if security.EncryptionEnabled() && !threadChecked {
			threadChecked = true
			var msg struct {
				Thread string `json:"thread"`
			}
			if err := json.Unmarshal(v, &msg); err == nil && msg.Thread != "" {
				sthr, terr := GetThread(msg.Thread)
				if terr == nil {
					var th struct {
						KMS struct {
							KeyID string `json:"key_id"`
						} `json:"kms"`
					}
					if json.Unmarshal([]byte(sthr), &th) == nil {
						threadKeyID = th.KMS.KeyID
					}
				}
			} else {
				return nil, fmt.Errorf("cannot determine thread for message version")
			}
		}
		if security.EncryptionEnabled() {
			if security.EncryptionHasFieldPolicy() {
				if threadKeyID == "" {
					return nil, fmt.Errorf("encryption enabled but no thread key available for message version")
				}
				var mm models.Message
				if err := json.Unmarshal(v, &mm); err != nil {
					return nil, fmt.Errorf("invalid message JSON: %w", err)
				}
				decBody, decErr := security.DecryptMessageBody(&mm, threadKeyID)
				if decErr != nil {
					return nil, fmt.Errorf("field decryption failed: %w", decErr)
				}
				mm.Body = decBody
				nb, err := json.Marshal(mm)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal decrypted message: %w", err)
				}
				v = nb
			} else {
				if threadKeyID == "" {
					return nil, fmt.Errorf("encryption enabled but no thread key available for message version")
				}
				var mm models.Message
				if err := json.Unmarshal(v, &mm); err == nil {
					b, derr := security.DecryptMessageBody(&mm, threadKeyID)
					if derr != nil {
						return nil, fmt.Errorf("decrypt failed: %w", derr)
					}
					mm.Body = b
					nb, merr := json.Marshal(mm)
					if merr != nil {
						return nil, fmt.Errorf("failed to marshal decrypted message: %w", merr)
					}
					v = nb
				} else {
					dec, err := kms.DecryptWithDEK(threadKeyID, v, nil)
					if err != nil {
						return nil, fmt.Errorf("decrypt failed: %w", err)
					}
					logger.Debug("decrypted_message_version", "threadKeyID", threadKeyID, "decrypted_len", len(dec))
					v = dec
				}
			}
		}
		out = append(out, string(v))
	}
	return out, iter.Error()
}

// returns latest version for message, error if not found
func GetLatestMessage(msgID string) (string, error) {
	tr := telemetry.Track("store.get_latest_message")
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
