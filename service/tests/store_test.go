package tests

import (
	"strings"
	"testing"
)

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
		k, err := storedb.MsgKey(c.threadID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("MsgKey error: %v", err)
		}
		tid, pts, pseq, perr := storedb.ParseMsgKey(k)
		if perr != nil {
			t.Fatalf("ParseMsgKey error: %v (key=%s)", perr, k)
		}
		if tid != c.threadID || pts != c.ts || pseq != c.seq {
			t.Fatalf("ParseMsgKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", tid, pts, pseq, c.threadID, c.ts, c.seq)
		}

		// VersionKey -> ParseVersionKey
		vk, err := storedb.VersionKey(c.msgID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("VersionKey error: %v", err)
		}
		mid, vts, vseq, verr := storedb.ParseVersionKey(vk)
		if verr != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", verr, vk)
		}
		if mid != c.msgID || vts != c.ts || vseq != c.seq {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", mid, vts, vseq, c.msgID, c.ts, c.seq)
		}

		// ThreadMetaKey
		mk, err := storedb.ThreadMetaKey(c.threadID)
		if err != nil {
			t.Fatalf("ThreadMetaKey error: %v", err)
		}
		if !strings.HasPrefix(mk, "thread:") || !strings.HasSuffix(mk, ":meta") {
			t.Fatalf("ThreadMetaKey malformed: %s", mk)
		}
	}

	// invalid ids
	if _, err := storedb.MsgKey("", 1, 1); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if _, err := storedb.VersionKey("", 1, 1); err == nil {
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
		k, err := storedb.MsgKey(c.threadID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("MsgKey error: %v", err)
		}
		tid, pts, pseq, err := storedb.ParseMsgKey(k)
		if err != nil {
			t.Fatalf("ParseMsgKey error: %v (key=%s)", err, k)
		}
		if tid != c.threadID || pts != c.ts || pseq != c.seq {
			t.Fatalf("ParseMsgKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", tid, pts, pseq, c.threadID, c.ts, c.seq)
		}

		// VersionKey round-trip
		vk, err := storedb.VersionKey(c.msgID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("VersionKey error: %v", err)
		}
		mid, vts, vseq, err := storedb.ParseVersionKey(vk)
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
	if p, err := storedb.MsgPrefix("abc"); err != nil || p != "thread:abc:msg:" {
		t.Fatalf("MsgPrefix unexpected: %v %v", p, err)
	}
	if p, err := storedb.ThreadPrefix("abc"); err != nil || p != "thread:abc:" {
		t.Fatalf("ThreadPrefix unexpected: %v %v", p, err)
	}

	// validators
	if err := storedb.ValidateThreadID(""); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if err := storedb.ValidateMsgID(""); err == nil {
		t.Fatalf("expected error for empty msg id")
	}
}
