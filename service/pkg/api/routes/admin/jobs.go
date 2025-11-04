package admin

import (
	"bytes"
	"encoding/base64"
	"encoding/json"

	"github.com/valyala/fasthttp"

	"progressdb/internal/retention"
	"progressdb/pkg/api/router"
	"progressdb/pkg/models"
	storedb "progressdb/pkg/store/db/storedb"
	"progressdb/pkg/store/encryption"
	thread_store "progressdb/pkg/store/features/threads"
	"progressdb/pkg/store/iterator/admin/ki"
	"progressdb/pkg/store/keys"
	"progressdb/pkg/store/pagination"
)

type EncryptThreadsRequest struct {
	// No parameters needed - process all threads
}

type EncryptThreadsResult struct {
	ThreadKey string `json:"thread_key"`
	Status    string `json:"status"` // "ok", "error", "skipped"
	Error     string `json:"error,omitempty"`
	DEKKeyID  string `json:"dek_key_id,omitempty"`
}

func encryptThread(threadKey string) EncryptThreadsResult {
	// Get thread data
	stored, err := thread_store.GetThreadData(threadKey)
	if err != nil {
		return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "thread not found"}
	}

	var th models.Thread
	if err := json.Unmarshal([]byte(stored), &th); err != nil {
		return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "invalid thread metadata"}
	}

	// Check if thread already has KMS
	if th.KMS != nil && th.KMS.KeyID != "" {
		return EncryptThreadsResult{ThreadKey: threadKey, Status: "skipped", Error: "already encrypted"}
	}

	// Create DEK for thread
	newKeyID, wrapped, kekID, kekVer, err := encryption.CreateDEK(threadKey)
	if err != nil {
		return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "create DEK failed: " + err.Error()}
	}

	// Update thread with KMS metadata
	th.KMS = &models.KMSMeta{
		KeyID:      newKeyID,
		WrappedDEK: base64.StdEncoding.EncodeToString(wrapped),
		KEKID:      kekID,
		KEKVersion: kekVer,
	}

	if payload, err := json.Marshal(th); err == nil {
		if err := saveThread(th.Key, string(payload)); err != nil {
			return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "failed to save thread metadata: " + err.Error()}
		}
	}

	// Encrypt existing messages
	prefix, err := keys.GenAllThreadMessagesPrefix(threadKey)
	if err != nil {
		return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "failed to generate message prefix: " + err.Error()}
	}

	iter, err := storedb.Iter()
	if err != nil {
		return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "failed to create iterator: " + err.Error()}
	}
	defer iter.Close()

	pfx := []byte(prefix)
	encryptedCount := 0

	for iter.SeekGE(pfx); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), pfx) {
			break
		}

		k := append([]byte(nil), iter.Key()...)
		v := append([]byte(nil), iter.Value()...)

		if encryption.LikelyJSON(v) {
			ct, _, err := encryption.EncryptWithDEK(th.KMS.KeyID, v, nil)
			if err != nil {
				return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "encryption failed: " + err.Error()}
			}

			// Backup original message
			backupKey := string(append([]byte(keys.BackupEncryptPrefix), k...))
			if err := storedb.SaveKey(backupKey, v); err != nil {
				return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "backup failed: " + err.Error()}
			}

			// Replace with encrypted message
			if err := storedb.Set(k, ct); err != nil {
				return EncryptThreadsResult{ThreadKey: threadKey, Status: "error", Error: "save encrypted message failed: " + err.Error()}
			}

			encryptedCount++
		}
	}

	return EncryptThreadsResult{
		ThreadKey: threadKey,
		Status:    "ok",
		DEKKeyID:  th.KMS.KeyID,
	}
}

func getAllThreads() ([]string, error) {
	// Use key iterator to get all thread metadata keys
	threadPrefix := keys.GenThreadMetadataPrefix()
	keyIter := ki.NewKeyIterator(storedb.Client)

	threadKeys, _, err := keyIter.ExecuteKeyQuery(threadPrefix, pagination.PaginationRequest{
		Limit: 10000, // Large limit to get all threads
	})
	if err != nil {
		return nil, err
	}

	// Parse thread keys from metadata keys
	var threadIDs []string
	for _, threadKey := range threadKeys {
		if parsed, err := keys.ParseKey(threadKey); err == nil && parsed.Type == keys.KeyTypeThread {
			threadIDs = append(threadIDs, parsed.ThreadKey)
		}
	}

	return threadIDs, nil
}

func EncryptThreads(ctx *fasthttp.RequestCtx) {
	var req EncryptThreadsRequest
	if err := json.NewDecoder(bytes.NewReader(ctx.PostBody())).Decode(&req); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusBadRequest, "invalid request")
		return
	}

	// Get all threads
	threadKeys, err := getAllThreads()
	if err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, "failed to get threads: "+err.Error())
		return
	}

	if len(threadKeys) == 0 {
		_ = router.WriteJSON(ctx, map[string]interface{}{
			"message": "no threads found",
			"results": []EncryptThreadsResult{},
		})
		return
	}

	// Process threads sequentially
	var results []EncryptThreadsResult
	successCount := 0
	skippedCount := 0
	errorCount := 0

	for _, tkey := range threadKeys {
		result := encryptThread(tkey)
		results = append(results, result)

		switch result.Status {
		case "ok":
			successCount++
		case "skipped":
			skippedCount++
		case "error":
			errorCount++
		}
	}

	_ = router.WriteJSON(ctx, map[string]interface{}{
		"summary": map[string]int{
			"total":   len(threadKeys),
			"success": successCount,
			"skipped": skippedCount,
			"errors":  errorCount,
		},
		"results": results,
	})
}

func RunRetentionCleanup(ctx *fasthttp.RequestCtx) {
	if err := retention.RunImmediate(); err != nil {
		router.WriteJSONError(ctx, fasthttp.StatusInternalServerError, err.Error())
		return
	}
	_ = router.WriteJSON(ctx, map[string]string{"status": "ok", "message": "retention run triggered"})
}
