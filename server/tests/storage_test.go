package tests

import (
	"context"
	"encoding/json"
	"testing"

	"progressdb/pkg/models"
	"progressdb/pkg/progressor"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"
    testutils "progressdb/tests/utils"
)

// TestProgressorInitializesLastSeq verifies that the progressor migration
// initializes thread.LastSeq from existing message keys.
func TestProgressorInitializesLastSeq(t *testing.T) {
	t.Helper()
	srv := testutils.SetupServer(t)
	defer srv.Close()

	// create a thread with LastSeq == 0
	var th models.Thread
	th.ID = utils.GenThreadID()
	th.Author = "author1"
	th.Title = "migration-test"
	th.CreatedTS = 1
	th.UpdatedTS = 1
	// intentionally leave LastSeq as zero to simulate older data
	b, _ := json.Marshal(th)
	if err := store.SaveThread(th.ID, string(b)); err != nil {
		t.Fatalf("SaveThread: %v", err)
	}

	// Insert two legacy-style message keys with seq parts 5 and 7.
	k1 := "thread:" + th.ID + ":msg:0000000000000001000-000005"
	v1 := []byte(`{"id":"m1","thread":"` + th.ID + `","ts":1000}`)
	if err := store.DBSet([]byte(k1), v1); err != nil {
		t.Fatalf("DBSet k1: %v", err)
	}
	k2 := "thread:" + th.ID + ":msg:0000000000000002000-000007"
	v2 := []byte(`{"id":"m2","thread":"` + th.ID + `","ts":2000}`)
	if err := store.DBSet([]byte(k2), v2); err != nil {
		t.Fatalf("DBSet k2: %v", err)
	}

	// run migration
	if err := progressor.Sync(context.Background(), "0.1.2", "0.2.0"); err != nil {
		t.Fatalf("progressor.Sync: %v", err)
	}

	// verify LastSeq updated to 7
	s, err := store.GetThread(th.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	var got models.Thread
	if err := json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatalf("unmarshal thread: %v", err)
	}
	if got.LastSeq != 7 {
		t.Fatalf("expected LastSeq=7; got %d", got.LastSeq)
	}
}
