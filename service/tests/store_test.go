package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"progressdb/pkg/models"
	"progressdb/pkg/store"
	"progressdb/pkg/utils"
	testutils "progressdb/tests/utils"
)

// TestProgressorInitializesLastSeq verifies that the progressor migration
// initializes thread.LastSeq from existing message keys.
func TestProgressorInitializesLastSeq(t *testing.T) {
	t.Helper()
	// Pre-seed the DB with legacy-style keys, then start the server process
	// which runs migrations on startup. Use PreseedDB to write raw keys.
	var th models.Thread
	th.ID = utils.GenThreadID()
	th.Author = "author1"
	th.Title = "migration-test"
	th.CreatedTS = 1
	th.UpdatedTS = 1

	workdir := testutils.PreseedDB(t, "progressor-init", func(storePath string) {
		// store is already opened by PreseedDB; write thread metadata and legacy keys.
		b, _ := json.Marshal(th)
		if err := store.SaveThread(th.ID, string(b)); err != nil {
			t.Fatalf("SaveThread: %v", err)
		}
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
	})

	// start server with pre-seeded DB so it will run progressor.Run on startup
	sp := testutils.StartServerProcessWithWorkdir(t, workdir, testutils.ServerOpts{})
	defer func() { _ = sp.Stop(t) }()

    // fetch thread via admin key endpoint (raw KV key) and verify LastSeq was initialized to 7
    var got models.Thread
    keyName := "thread:" + th.ID + ":meta"
    status, body := testutils.GetAdminKey(t, sp.Addr, keyName)
    if status != 200 {
        t.Fatalf("expected 200 fetching thread meta via admin key; got %d", status)
    }
    if err := json.Unmarshal(body, &got); err != nil {
        t.Fatalf("failed to decode thread meta: %v (body=%s)", err, string(body))
    }
	if got.LastSeq != 7 {
		t.Fatalf("expected LastSeq=7; got %d", got.LastSeq)
	}
}

// verifies MsgKey/VersionKey/ThreadMetaKey builders and parsers.
func TestKeysBuildersParsers(t *testing.T) {
	t.Helper()

	cases := []struct {
		threadID string
		msgID    string
		ts       int64
		seq      uint64
	}{
		{threadID: "thread-1", msgID: "msg-1", ts: 12345, seq: 1},
		{threadID: "t_ABC.123", msgID: "m.X_Y-9", ts: 0, seq: 0},
		{threadID: "a1b2c3", msgID: "z9", ts: 9999999999, seq: 42},
	}

	for _, c := range cases {
		// MsgKey -> ParseMsgKey
		k, err := store.MsgKey(c.threadID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("MsgKey error: %v", err)
		}
		tid, pts, pseq, perr := store.ParseMsgKey(k)
		if perr != nil {
			t.Fatalf("ParseMsgKey error: %v (key=%s)", perr, k)
		}
		if tid != c.threadID || pts != c.ts || pseq != c.seq {
			t.Fatalf("ParseMsgKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", tid, pts, pseq, c.threadID, c.ts, c.seq)
		}

		// VersionKey -> ParseVersionKey
		vk, err := store.VersionKey(c.msgID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("VersionKey error: %v", err)
		}
		mid, vts, vseq, verr := store.ParseVersionKey(vk)
		if verr != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", verr, vk)
		}
		if mid != c.msgID || vts != c.ts || vseq != c.seq {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", mid, vts, vseq, c.msgID, c.ts, c.seq)
		}

		// ThreadMetaKey
		mk, err := store.ThreadMetaKey(c.threadID)
		if err != nil {
			t.Fatalf("ThreadMetaKey error: %v", err)
		}
		if !strings.HasPrefix(mk, "thread:") || !strings.HasSuffix(mk, ":meta") {
			t.Fatalf("ThreadMetaKey malformed: %s", mk)
		}
	}

	// invalid ids
	if _, err := store.MsgKey("", 1, 1); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if _, err := store.VersionKey("", 1, 1); err == nil {
		t.Fatalf("expected error for empty msg id")
	}
}

// Additional key helper tests appended from package store helpers.
func TestKeysRoundTrip_StoreHelpers(t *testing.T) {
	cases := []struct {
		threadID string
		msgID    string
		ts       int64
		seq      uint64
	}{
		{threadID: "t1", msgID: "m1", ts: 1, seq: 1},
		{threadID: "a_b.C-123", msgID: "id.XYZ", ts: 0, seq: 0},
		{threadID: "z9", msgID: "m-9", ts: 9999999999, seq: 42},
	}

	for _, c := range cases {
		// MsgKey round-trip
		k, err := store.MsgKey(c.threadID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("MsgKey error: %v", err)
		}
		tid, pts, pseq, err := store.ParseMsgKey(k)
		if err != nil {
			t.Fatalf("ParseMsgKey error: %v (key=%s)", err, k)
		}
		if tid != c.threadID || pts != c.ts || pseq != c.seq {
			t.Fatalf("ParseMsgKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", tid, pts, pseq, c.threadID, c.ts, c.seq)
		}

		// VersionKey round-trip
		vk, err := store.VersionKey(c.msgID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("VersionKey error: %v", err)
		}
		mid, vts, vseq, err := store.ParseVersionKey(vk)
		if err != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", err, vk)
		}
		if mid != c.msgID || vts != c.ts || vseq != c.seq {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", mid, vts, vseq, c.msgID, c.ts, c.seq)
		}
	}
}

func TestPrefixesAndValidators_StoreHelpers(t *testing.T) {
	// Prefix helpers
	if p, err := store.MsgPrefix("abc"); err != nil || p != "thread:abc:msg:" {
		t.Fatalf("MsgPrefix unexpected: %v %v", p, err)
	}
	if p, err := store.ThreadPrefix("abc"); err != nil || p != "thread:abc:" {
		t.Fatalf("ThreadPrefix unexpected: %v %v", p, err)
	}

	// validators
	if err := store.ValidateThreadID(""); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if err := store.ValidateMsgID(""); err == nil {
		t.Fatalf("expected error for empty msg id")
	}
}
