package encryption

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"progressdb/pkg/kms"
	"progressdb/pkg/models"
	"progressdb/pkg/security"
	storedb "progressdb/pkg/store/db/store"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/threads"

	"github.com/cockroachdb/pebble"
)

// migrates all thread messages to new DEK; backs up old data before overwriting
func RotateThreadDEK(threadID, newKeyID string) error {
	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	oldKeyID := ""
	if s, err := threads.GetThread(threadID); err == nil {
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
	mp, merr := keys.MsgPrefix(threadID)
	if merr != nil {
		return merr
	}
	prefix := []byte(mp)
	iter, err := storedb.Client.NewIter(&pebble.IterOptions{})
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
		if LikelyJSON(v) {
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
				if err := storedb.Client.Set(backupKey, v, storedb.WriteOpt(true)); err != nil {
					return fmt.Errorf("backup failed: %w", err)
				}
				if err := storedb.Client.Set(k, nb, storedb.WriteOpt(true)); err != nil {
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
		if err := storedb.Client.Set(backupKey, v, storedb.WriteOpt(true)); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
		if err := storedb.Client.Set(k, ct, storedb.WriteOpt(true)); err != nil {
			return fmt.Errorf("write new ciphertext failed: %w", err)
		}
	}
	if s, terr := threads.GetThread(threadID); terr == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			if th.KMS == nil {
				th.KMS = &models.KMSMeta{}
			}
			th.KMS.KeyID = newKeyID
			if nb, merr := json.Marshal(th); merr == nil {
				if err := threads.SaveThread(th.ID, string(nb)); err != nil {
					return fmt.Errorf("save thread key mapping failed: %w", err)
				}
			}
		}
	}
	return iter.Error()
}
