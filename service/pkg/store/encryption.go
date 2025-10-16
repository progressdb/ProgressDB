package store

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"progressdb/pkg/kms"
	"progressdb/pkg/models"
	"progressdb/pkg/security"

	"github.com/cockroachdb/pebble"
)

// migrates all thread messages to new DEK; backs up old data before overwriting
func RotateThreadDEK(threadID, newKeyID string) error {
	if db == nil {
		return fmt.Errorf("pebble not opened; call store.Open first")
	}
	oldKeyID := ""
	if s, err := GetThread(threadID); err == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			if th.KMS != nil {
				oldKeyID = th.KMS.KeyID
			}
		}
	}
	if oldKeyID == newKeyID {
		return nil
	}
	mp, merr := MsgPrefix(threadID)
	if merr != nil {
		return merr
	}
	prefix := []byte(mp)
	iter, err := db.NewIter(&pebble.IterOptions{})
	if err != nil {
		return err
	}
	defer iter.Close()

	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		k := append([]byte(nil), iter.Key()...)
		v := append([]byte(nil), iter.Value()...)
		if likelyJSON(v) {
			var mm models.Message
			if err := json.Unmarshal(v, &mm); err == nil {
				decBody, derr := security.DecryptMessageBody(&mm, oldKeyID)
				if derr != nil {
					return fmt.Errorf("decrypt message failed: %w", derr)
				}
				pt, merr := json.Marshal(decBody)
				if merr != nil {
					return fmt.Errorf("marshal plaintext failed: %w", merr)
				}
				ct, _, eerr := kms.EncryptWithDEK(newKeyID, pt, nil)
				for i := range pt {
					pt[i] = 0
				}
				if eerr != nil {
					return fmt.Errorf("encrypt with new key failed: %w", eerr)
				}
				mm.Body = map[string]interface{}{"_enc": "gcm", "v": base64.StdEncoding.EncodeToString(ct)}
				nb, merr := json.Marshal(mm)
				if merr != nil {
					return fmt.Errorf("failed to marshal migrated message: %w", merr)
				}
				backupKey := append([]byte("backup:migrate:"), k...)
				if err := db.Set(backupKey, v, writeOpt(true)); err != nil {
					return fmt.Errorf("backup failed: %w", err)
				}
				if err := db.Set(k, nb, writeOpt(true)); err != nil {
					return fmt.Errorf("write new ciphertext failed: %w", err)
				}
				continue
			}
		}
		pt, derr := kms.DecryptWithDEK(oldKeyID, v, nil)
		if derr != nil {
			return fmt.Errorf("decrypt message failed: %w", derr)
		}
		ct, _, eerr := kms.EncryptWithDEK(newKeyID, pt, nil)
		for i := range pt {
			pt[i] = 0
		}
		if eerr != nil {
			return fmt.Errorf("encrypt with new key failed: %w", eerr)
		}
		backupKey := append([]byte("backup:migrate:"), k...)
		if err := db.Set(backupKey, v, writeOpt(true)); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
		if err := db.Set(k, ct, writeOpt(true)); err != nil {
			return fmt.Errorf("write new ciphertext failed: %w", err)
		}
	}
	if s, terr := GetThread(threadID); terr == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			if th.KMS == nil {
				th.KMS = &models.KMSMeta{}
			}
			th.KMS.KeyID = newKeyID
			if nb, merr := json.Marshal(th); merr == nil {
				if err := SaveThread(th.ID, string(nb)); err != nil {
					return fmt.Errorf("save thread key mapping failed: %w", err)
				}
			}
		}
	}
	return iter.Error()
}

// true if likely contains JSON object/array based on first non-whitespace
func likelyJSON(b []byte) bool {
	for _, c := range b {
		if c == ' ' || c == '\n' || c == '\r' || c == '\t' {
			continue
		}
		return c == '{' || c == '['
	}
	return false
}

// exported version of likelyJSON
func LikelyJSON(b []byte) bool { return likelyJSON(b) }
