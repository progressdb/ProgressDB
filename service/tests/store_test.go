package tests

import (
	"strings"
	"testing"

	"progressdb/pkg/store/keys"
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
		k, err := keys.GenMsgKey(c.threadID, "test-msg", c.seq)
		if err != nil {
			t.Fatalf("GenMsgKey error: %v", err)
		}
		tid, msgID, pseq, perr := keys.ParseMsgKey(k)
		if perr != nil {
			t.Fatalf("ParseMsgKey error: %v (key=%s)", perr, k)
		}
		if tid != c.threadID || msgID != "test-msg" || pseq != c.seq {
			t.Fatalf("ParseMsgKey mismatch: got (%s,%s,%d) want (%s,%s,%d)", tid, msgID, pseq, c.threadID, "test-msg", c.seq)
		}

		// VersionKey -> ParseVersionKey
		vk, err := keys.GenVersionKey(c.msgID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("VersionKey error: %v", err)
		}
		mid, vts, vseq, verr := keys.ParseVersionKey(vk)
		if verr != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", verr, vk)
		}
		if mid != c.msgID || vts != c.ts || vseq != c.seq {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", mid, vts, vseq, c.msgID, c.ts, c.seq)
		}

		// ThreadMetaKey
		mk, err := keys.GenThreadMetaKey(c.threadID)
		if err != nil {
			t.Fatalf("ThreadMetaKey error: %v", err)
		}
		if !strings.HasPrefix(mk, "thread:") || !strings.HasSuffix(mk, ":meta") {
			t.Fatalf("ThreadMetaKey malformed: %s", mk)
		}
	}

	// invalid ids
	if _, err := keys.GenMsgKey("", "test-msg", 1); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if _, err := keys.GenVersionKey("", 1, 1); err == nil {
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
		k, err := keys.GenMsgKey(c.threadID, "test-msg", c.seq)
		if err != nil {
			t.Fatalf("MsgKey error: %v", err)
		}
		tid, pts, pseq, err := keys.ParseMsgKey(k)
		if err != nil {
			t.Fatalf("ParseMsgKey error: %v (key=%s)", err, k)
		}
		if tid != c.threadID || pts != c.ts || pseq != c.seq {
			t.Fatalf("ParseMsgKey mismatch: got (%s,%d,%d) want (%s,%d,%d)", tid, pts, pseq, c.threadID, c.ts, c.seq)
		}

		// VersionKey round-trip
		vk, err := keys.GenVersionKey(c.msgID, c.ts, c.seq)
		if err != nil {
			t.Fatalf("VersionKey error: %v", err)
		}
		mid, vts, vseq, err := keys.ParseVersionKey(vk)
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
	if p, err := keys.GenMsgPrefix("abc"); err != nil || p != "thread:abc:msg:" {
		t.Fatalf("MsgPrefix unexpected: %v %v", p, err)
	}
	if p, err := keys.GenThreadPrefix("abc"); err != nil || p != "thread:abc:" {
		t.Fatalf("ThreadPrefix unexpected: %v %v", p, err)
	}

	// validators
	if err := keys.ValidateThreadID(""); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if err := keys.ValidateMsgID(""); err == nil {
		t.Fatalf("expected error for empty msg id")
	}
}
