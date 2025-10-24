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
		// MessageKey -> ParseMessageKey
		k := keys.GenMessageKey(c.threadID, "test-msg", c.seq)
		parts, perr := keys.ParseMessageKey(k)
		if perr != nil {
			t.Fatalf("ParseMessageKey error: %v (key=%s)", perr, k)
		}
		if parts.ThreadID != c.threadID || parts.MsgID != "test-msg" || parts.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseMessageKey mismatch: got (%s,%s,%s) want (%s,%s,%s)", parts.ThreadID, parts.MsgID, parts.Seq, c.threadID, "test-msg", keys.PadSeq(c.seq))
		}

		// VersionKey -> ParseVersionKey
		vk := keys.GenVersionKey(c.msgID, c.ts, c.seq)
		vparts, verr := keys.ParseVersionKey(vk)
		if verr != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", verr, vk)
		}
		if vparts.MsgID != c.msgID || vparts.TS != keys.PadTS(c.ts) || vparts.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%s,%s) want (%s,%s,%s)", vparts.MsgID, vparts.TS, vparts.Seq, c.msgID, keys.PadTS(c.ts), keys.PadSeq(c.seq))
		}

		// ThreadKey
		mk := keys.GenThreadKey(c.threadID)
		if !strings.HasPrefix(mk, "t:") || !strings.HasSuffix(mk, ":meta") {
			t.Fatalf("ThreadKey malformed: %s", mk)
		}
	}

	// invalid ids
	if err := keys.ValidateThreadID(""); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if err := keys.ValidateMsgID(""); err == nil {
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
		// MessageKey round-trip
		k := keys.GenMessageKey(c.threadID, "test-msg", c.seq)
		parts, err := keys.ParseMessageKey(k)
		if err != nil {
			t.Fatalf("ParseMessageKey error: %v (key=%s)", err, k)
		}
		parsedSeq, _ := keys.ParseKeySequence(parts.Seq)
		if parts.ThreadID != c.threadID || parsedSeq != c.seq {
			t.Fatalf("ParseMessageKey mismatch: got (%s,%s) want (%s,%d)", parts.ThreadID, parts.Seq, c.threadID, c.seq)
		}

		// VersionKey round-trip
		vk := keys.GenVersionKey(c.msgID, c.ts, c.seq)
		vparts, err := keys.ParseVersionKey(vk)
		if err != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", err, vk)
		}
		parsedTS, _ := keys.ParseKeyTimestamp(vparts.TS)
		parsedSeq, _ = keys.ParseKeySequence(vparts.Seq)
		if vparts.MsgID != c.msgID || parsedTS != c.ts || parsedSeq != c.seq {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%s,%s) want (%s,%d,%d)", vparts.MsgID, vparts.TS, vparts.Seq, c.msgID, c.ts, c.seq)
		}
	}
}

func TestPrefixesAndValidators_StoreHelpers(t *testing.T) {
	// Prefix helpers - using thread message start as equivalent
	if p := keys.GenThreadMessageStart("abc"); p != "idx:t:abc:ms:start" {
		t.Fatalf("ThreadMessageStart unexpected: %s", p)
	}
	if p := keys.GenThreadKey("abc"); p != "t:abc:meta" {
		t.Fatalf("ThreadKey unexpected: %s", p)
	}

	// validators
	if err := keys.ValidateThreadID(""); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if err := keys.ValidateMsgID(""); err == nil {
		t.Fatalf("expected error for empty msg id")
	}
}
