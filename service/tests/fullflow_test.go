//go:build integration
// +build integration

package tests

import (
	"encoding/json"
	"testing"
	"time"

	utils "progressdb/tests/utils"
)

func TestE2E_ProvisionThenRotateThenRead(t *testing.T) {
	sp := utils.StartTestServerProcess(t)
	defer func() { _ = sp.Stop(t) }()
	user := "e2e"
	tid, _ := utils.CreateThreadAndWait(t, sp.Addr, user, "e2e-thread", 5*time.Second)
	msg := map[string]interface{}{"author": user, "body": map[string]string{"text": "before-rotate"}, "thread": tid}
	_ = utils.CreateMessageAndWait(t, sp.Addr, user, tid, msg, 5*time.Second)
	var rout map[string]string
	status := utils.AdminPostJSON(t, sp.Addr, "/admin/encryption/rotate-thread-dek", map[string]string{"thread_key": tid}, &rout)
	if status != 200 {
		t.Fatalf("rotate failed: %d", status)
	}
	time.Sleep(100 * time.Millisecond)
	var lob struct {
		Messages []map[string]interface{} `json:"messages"`
	}
	lstatus := utils.FrontendGetJSON(t, sp.Addr, "/v1/threads/"+tid+"/messages", user, &lob)
	if lstatus != 200 {
		t.Fatalf("list messages failed: %d", lstatus)
	}
	if len(lob.Messages) == 0 {
		t.Fatalf("expected messages after rotate")
	}
}
