//go:build integration
// +build integration
package tests

import (
    "bytes"
    "encoding/json"
    "net/http"
    "testing"

    "progressdb/pkg/kms"
    "progressdb/pkg/store"
    utils "progressdb/tests/utils"
)

func TestEncryption_RotateThreadDEK(t *testing.T) {
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

    // Create a new DEK for the thread and rotate
    newKeyID, _, _, _, err := kms.CreateDEKForThread(thread)
    if err != nil {
        t.Fatalf("CreateDEKForThread: %v", err)
    }
    if err := store.RotateThreadDEK(thread, newKeyID); err != nil {
        t.Fatalf("RotateThreadDEK failed: %v", err)
    }
}

