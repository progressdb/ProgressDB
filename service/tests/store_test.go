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
		// MessageKey -> ParseKey (unified parser)
		k := keys.GenMessageKey(c.threadID, "test-msg", c.seq)
		parsed, perr := keys.ParseKey(k)
		if perr != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", perr, k)
		}
		if parsed.Type != keys.KeyTypeMessage || parsed.ThreadTS != c.threadID || parsed.MessageTS != "test-msg" || parsed.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s,%s) want (%s,%s,%s,%s)", parsed.Type, parsed.ThreadTS, parsed.MessageTS, parsed.Seq, keys.KeyTypeMessage, c.threadID, "test-msg", keys.PadSeq(c.seq))
		}

		// VersionKey -> ParseKey (unified parser)
		vk := keys.GenVersionKey(c.msgID, c.ts, c.seq)
		vparsed, verr := keys.ParseKey(vk)
		if verr != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", verr, vk)
		}
		if vparsed.Type != keys.KeyTypeVersion || vparsed.MessageTS != c.msgID || vparsed.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s) want (%s,%d,%d)", vparsed.Type, vparsed.MessageTS, vparsed.Seq, keys.KeyTypeVersion, c.msgID, c.seq)
		}
		vparts, verr := keys.ParseVersionKey(vk)
		if verr != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", verr, vk)
		}
		if vparts.MessageKey != c.msgID || vparts.MessageTS != keys.PadTS(c.ts) || vparts.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%s,%s) want (%s,%s,%s)", vparts.MessageKey, vparts.MessageTS, vparts.Seq, c.msgID, keys.PadTS(c.ts), keys.PadSeq(c.seq))
		}

		// ThreadKey
		mk := keys.GenThreadKey(c.threadID)
		if !strings.HasPrefix(mk, "t:") || !strings.HasSuffix(mk, ":meta") {
			t.Fatalf("ThreadKey malformed: %s", mk)
		}
	}

	// invalid ids
	if err := keys.ValidateThreadKey(""); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if err := keys.ValidateMessageKey(""); err == nil {
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
		parsed, err := keys.ParseKey(k)
		if err != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", err, k)
		}
		parsedSeq, _ := keys.ParseKeySequence(parsed.Seq)
		if parsed.Type != keys.KeyTypeMessage || parsed.ThreadTS != c.threadID || parsedSeq != c.seq {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s) want (%s,%s,%d)", parsed.Type, parsed.ThreadTS, parsed.Seq, keys.KeyTypeMessage, c.threadID, c.seq)
		}

		// VersionKey round-trip
		vk := keys.GenVersionKey(c.msgID, c.ts, c.seq)
		vparsed, err := keys.ParseKey(vk)
		if err != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", err, vk)
		}
		parsedTS, _ := keys.ParseKeyTimestamp(vparsed.MessageTS)
		parsedSeq, _ := keys.ParseKeySequence(vparsed.Seq)
		if vparsed.Type != keys.KeyTypeVersion || vparsed.MessageTS != c.msgID || parsedTS != c.ts || parsedSeq != c.seq {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s,%s,%d,%d) want (%s,%s,%d,%d)", vparsed.Type, vparsed.MessageTS, parsedTS, parsedSeq, keys.KeyTypeVersion, c.msgID, c.ts, c.seq)
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
	if err := keys.ValidateThreadKey(""); err == nil {
		t.Fatalf("expected error for empty thread id")
	}
	if err := keys.ValidateMessageKey(""); err == nil {
		t.Fatalf("expected error for empty msg id")
	}
}
