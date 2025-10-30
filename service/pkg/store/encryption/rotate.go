package encryption

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/encryption/kms"
	"progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/keys"

	"github.com/cockroachdb/pebble"
)

func RotateThreadDEK(threadKey string, newKeyID string) error {
	if storedb.Client == nil {
		return fmt.Errorf("pebble not opened; call storedb.Open first")
	}
	oldKeyID := ""
	if s, err := threads.GetThread(threadKey); err == nil {
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
	threadPrefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return fmt.Errorf("failed to generate thread prefix: %w", err)
	}
	lowerBound := []byte(threadPrefix)
	upperBound := calculateUpperBound(threadPrefix)

	iter, err := storedb.Client.NewIter(&pebble.IterOptions{
		LowerBound: lowerBound,
		UpperBound: upperBound,
	})
	if err != nil {
		return fmt.Errorf("failed to create iterator: %w", err)
	}
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		k := append([]byte(nil), iter.Key()...)
		v := append([]byte(nil), iter.Value()...)
		if LikelyJSON(v) {
			var mm models.Message
			if err := json.Unmarshal(v, &mm); err == nil {
				decBody, derr := DecryptMessageBody(&mm, oldKeyID)
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
	if s, terr := threads.GetThread(threadKey); terr == nil {
		var th models.Thread
		if err := json.Unmarshal([]byte(s), &th); err == nil {
			if th.KMS == nil {
				th.KMS = &models.KMSMeta{}
			}
			th.KMS.KeyID = newKeyID
			if nb, merr := json.Marshal(th); merr == nil {
				threadKeyStr := keys.GenThreadKey(threadKey)
				if err := storedb.Client.Set([]byte(threadKeyStr), nb, storedb.WriteOpt(true)); err != nil {
					return fmt.Errorf("save thread key mapping failed: %w", err)
				}
			}
		}
	}
	return iter.Error()
}

func calculateUpperBound(prefix string) []byte {
	prefixBytes := []byte(prefix)
	upper := make([]byte, len(prefixBytes))
	copy(upper, prefixBytes)

	for i := len(upper) - 1; i >= 0; i-- {
		if upper[i] < 0xFF {
			upper[i]++
			return upper
		}
		upper[i] = 0
	}

	return append(prefixBytes, 0xFF)
}
