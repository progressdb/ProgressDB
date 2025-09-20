package units

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"progressdb/pkg/kms"
	"progressdb/pkg/models"
	"progressdb/pkg/store"
	utils "progressdb/tests/utils"
)

// TestRotateThreadDEK verifies RotateThreadDEK works for JSON-embedded
// encrypted bodies and for legacy raw-ciphertext entries.
func TestRotateThreadDEK(t *testing.T) {
	srv := utils.SetupServer(t)
	defer srv.Close()

	user := "rotator"
	sig := utils.SignHMAC("signsecret", user)

	// Create a message (server will create a thread and provision a DEK)
	payload := map[string]interface{}{"body": map[string]string{"text": "rotate-me"}}
	b, _ := json.Marshal(payload)
	creq, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b))
	creq.Header.Set("X-User-ID", user)
	creq.Header.Set("X-User-Signature", sig)
	cres, err := http.DefaultClient.Do(creq)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}
	defer cres.Body.Close()
	var cout map[string]interface{}
	if err := json.NewDecoder(cres.Body).Decode(&cout); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	thread := cout["thread"].(string)

	// Read current message via API to capture decrypted body
	lreq, _ := http.NewRequest("GET", srv.URL+"/v1/messages?thread="+thread, nil)
	lreq.Header.Set("X-User-ID", user)
	lreq.Header.Set("X-User-Signature", sig)
	lres, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list request failed: %v", err)
	}
	defer lres.Body.Close()
	var lout map[string]interface{}
	if err := json.NewDecoder(lres.Body).Decode(&lout); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	msgs := lout["messages"].([]interface{})
	if len(msgs) == 0 {
		t.Fatalf("no messages returned")
	}
	before := msgs[0].(map[string]interface{})["body"].(map[string]interface{})["text"].(string)

	// Create a new DEK for the thread and rotate
	newKeyID, _, _, _, err := kms.CreateDEKForThread(thread)
	if err != nil {
		t.Fatalf("CreateDEKForThread: %v", err)
	}
	if err := store.RotateThreadDEK(thread, newKeyID); err != nil {
		t.Fatalf("RotateThreadDEK failed: %v", err)
	}

	// Read back via API and confirm body unchanged
	lres2, err := http.DefaultClient.Do(lreq)
	if err != nil {
		t.Fatalf("list request after rotate failed: %v", err)
	}
	defer lres2.Body.Close()
	var lout2 map[string]interface{}
	if err := json.NewDecoder(lres2.Body).Decode(&lout2); err != nil {
		t.Fatalf("decode list response 2: %v", err)
	}
	msgs2 := lout2["messages"].([]interface{})
	after := msgs2[0].(map[string]interface{})["body"].(map[string]interface{})["text"].(string)
	if before != after {
		t.Fatalf("body changed after rotate: before=%q after=%q", before, after)
	}

	// --- legacy raw-ciphertext scenario ---
	// Create another message (new thread)
	payload2 := map[string]interface{}{"body": map[string]string{"text": "legacy-raw"}}
	b2, _ := json.Marshal(payload2)
	creq2, _ := http.NewRequest("POST", srv.URL+"/v1/messages", bytes.NewReader(b2))
	creq2.Header.Set("X-User-ID", user)
	creq2.Header.Set("X-User-Signature", sig)
	cres2, err := http.DefaultClient.Do(creq2)
	if err != nil {
		t.Fatalf("create2 failed: %v", err)
	}
	defer cres2.Body.Close()
	var cout2 map[string]interface{}
	_ = json.NewDecoder(cres2.Body).Decode(&cout2)
	thread2 := cout2["thread"].(string)

	// Find DB key for the message and replace stored value with raw ciphertext
	iter, err := store.DBIter()
	if err != nil {
		t.Fatalf("DBIter: %v", err)
	}
	defer iter.Close()
	prefix := []byte("thread:" + thread2 + ":msg:")
	var dbKey []byte
	var rawPlain []byte
	for iter.SeekGE(prefix); iter.Valid(); iter.Next() {
		if !bytes.HasPrefix(iter.Key(), prefix) {
			break
		}
		dbKey = append([]byte(nil), iter.Key()...)
		v := append([]byte(nil), iter.Value()...)
		// unmarshal to get body plaintext
		var mm models.Message
		if err := json.Unmarshal(v, &mm); err != nil {
			continue
		}
		rawPlain, _ = json.Marshal(mm.Body)
		break
	}
	if dbKey == nil {
		t.Fatalf("could not locate message DB key for thread %s", thread2)
	}

	// get oldKeyID
	s, err := store.GetThread(thread2)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	var th models.Thread
	if err := json.Unmarshal([]byte(s), &th); err != nil {
		t.Fatalf("unmarshal thread: %v", err)
	}
	oldKeyID := th.KMS.KeyID

	// encrypt plaintext with old DEK and overwrite DB entry with raw ciphertext
	ct, _, err := kms.EncryptWithDEK(oldKeyID, rawPlain, nil)
	if err != nil {
		t.Fatalf("EncryptWithDEK raw: %v", err)
	}
	if err := store.DBSet(dbKey, ct); err != nil {
		t.Fatalf("DBSet raw ct: %v", err)
	}

	// Create another new DEK and rotate
	newKeyID2, _, _, _, err := kms.CreateDEKForThread(thread2)
	if err != nil {
		t.Fatalf("CreateDEKForThread 2: %v", err)
	}
	if err := store.RotateThreadDEK(thread2, newKeyID2); err != nil {
		t.Fatalf("RotateThreadDEK legacy failed: %v", err)
	}

	// After rotate, read DB entry and decrypt with newKeyID2 to verify plaintext
	iter2, err := store.DBIter()
	if err != nil {
		t.Fatalf("DBIter2: %v", err)
	}
	defer iter2.Close()
	var newCt []byte
	for iter2.SeekGE(prefix); iter2.Valid(); iter2.Next() {
		if !bytes.HasPrefix(iter2.Key(), prefix) {
			break
		}
		newCt = append([]byte(nil), iter2.Value()...)
		break
	}
	if newCt == nil {
		t.Fatalf("could not find migrated ciphertext")
	}
	pt2, err := kms.DecryptWithDEK(newKeyID2, newCt, nil)
	if err != nil {
		t.Fatalf("decrypt after rotate failed: %v", err)
	}
	// compare plaintext bytes (as JSON) to original rawPlain
	if !bytes.Equal(pt2, rawPlain) {
		t.Fatalf("rotated plaintext mismatch")
	}
}
