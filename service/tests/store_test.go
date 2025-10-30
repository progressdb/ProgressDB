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
		threadKey  string
		messageKey string
		ts         int64
		seq        uint64
	}{
		{threadKey: "thread-1", messageKey: "msg-1", ts: 12345, seq: 1},
		{threadKey: "t_ABC.123", messageKey: "m.X_Y-9", ts: 0, seq: 0},
		{threadKey: "a1b2c3", messageKey: "z9", ts: 9999999999, seq: 42},
	}

	for _, c := range cases {
		// MessageKey -> ParseKey (unified parser)
		k := keys.GenMessageKey(c.threadKey, "test-msg", c.seq)
		parsed, perr := keys.ParseKey(k)
		if perr != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", perr, k)
		}
		if parsed.Type != keys.KeyTypeMessage || parsed.ThreadTS != c.threadKey || parsed.MessageTS != "test-msg" || parsed.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s,%s) want (%s,%s,%s,%s)", parsed.Type, parsed.ThreadTS, parsed.MessageTS, parsed.Seq, keys.KeyTypeMessage, c.threadKey, "test-msg", keys.PadSeq(c.seq))
		}

		// VersionKey -> ParseKey (unified parser)
		vk := keys.GenVersionKey(c.messageKey, c.ts, c.seq)
		vparsed, verr := keys.ParseKey(vk)
		if verr != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", verr, vk)
		}
		if vparsed.Type != keys.KeyTypeVersion || vparsed.MessageTS != c.messageKey || vparsed.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s) want (%s,%s,%d)", vparsed.Type, vparsed.MessageTS, vparsed.Seq, keys.KeyTypeVersion, c.messageKey, c.seq)
		}
		vparts, verr := keys.ParseVersionKey(vk)
		if verr != nil {
			t.Fatalf("ParseVersionKey error: %v (key=%s)", verr, vk)
		}
		if vparts.MessageKey != c.messageKey || vparts.MessageTS != keys.PadTS(c.ts) || vparts.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseVersionKey mismatch: got (%s,%s,%s) want (%s,%s,%s)", vparts.MessageKey, vparts.MessageTS, vparts.Seq, c.messageKey, keys.PadTS(c.ts), keys.PadSeq(c.seq))
		}

		// ThreadKey
		mk := keys.GenThreadKey(c.threadKey)
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
		threadKey  string
		messageKey string
		ts         int64
		seq        uint64
	}{
		{threadKey: "t1", messageKey: "m1", ts: 1, seq: 1},
		{threadKey: "a_b.C-123", messageKey: "id.XYZ", ts: 0, seq: 0},
		{threadKey: "z9", messageKey: "m-9", ts: 9999999999, seq: 42},
	}

	for _, c := range cases {
		// MessageKey round-trip
		k := keys.GenMessageKey(c.threadKey, "test-msg", c.seq)
		parsed, err := keys.ParseKey(k)
		if err != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", err, k)
		}
		if parsed.Type != keys.KeyTypeMessage || parsed.ThreadTS != c.threadKey || parsed.MessageTS != "test-msg" || parsed.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s,%s) want (%s,%s,%s,%s)", parsed.Type, parsed.ThreadTS, parsed.MessageTS, parsed.Seq, keys.KeyTypeMessage, c.threadKey, "test-msg", keys.PadSeq(c.seq))
		}

		// VersionKey round-trip
		vk := keys.GenVersionKey(c.messageKey, c.ts, c.seq)
		vparsed, err := keys.ParseKey(vk)
		if err != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", err, vk)
		}
		if vparsed.Type != keys.KeyTypeVersion || vparsed.MessageTS != c.messageKey || vparsed.Seq != keys.PadSeq(c.seq) {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s) want (%s,%s,%d)", vparsed.Type, vparsed.MessageTS, vparsed.Seq, keys.KeyTypeVersion, c.messageKey, c.seq)
		}
		msgSeq, _ := keys.ParseKeySequence(parsed.Seq)
		if parsed.Type != keys.KeyTypeMessage || parsed.ThreadTS != c.threadKey || msgSeq != c.seq {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%s) want (%s,%s,%d)", parsed.Type, parsed.ThreadTS, parsed.Seq, keys.KeyTypeMessage, c.threadKey, c.seq)
		}

		// VersionKey round-trip
		vk = keys.GenVersionKey(c.messageKey, c.ts, c.seq)
		vparsed, err = keys.ParseKey(vk)
		if err != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", err, vk)
		}
		verTS, _ := keys.ParseKeyTimestamp(vparsed.MessageTS)
		verSeq, _ := keys.ParseKeySequence(vparsed.Seq)
		if vparsed.Type != keys.KeyTypeVersion || vparsed.MessageTS != c.messageKey || verTS != c.ts || verSeq != c.seq {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%d,%d,%d) want (%s,%s,%d,%d)", vparsed.Type, vparsed.MessageTS, verTS, verSeq, keys.KeyTypeVersion, c.messageKey, c.ts, c.seq)
		}

		// VersionKey round-trip
		vk = keys.GenVersionKey(c.messageKey, c.ts, c.seq)
		vparsed, err = keys.ParseKey(vk)
		if err != nil {
			t.Fatalf("ParseKey error: %v (key=%s)", err, vk)
		}
		var parsedTS int64
		parsedTS, _ = keys.ParseKeyTimestamp(vparsed.MessageTS)
		var parsedSeq uint64
		parsedSeq, _ = keys.ParseKeySequence(vparsed.Seq)
		if vparsed.Type != keys.KeyTypeVersion || vparsed.MessageTS != c.messageKey || parsedTS != c.ts || parsedSeq != c.seq {
			t.Fatalf("ParseKey mismatch: got (%s,%s,%d,%d,%d) want (%s,%s,%d,%d)", vparsed.Type, vparsed.MessageTS, parsedTS, parsedSeq, keys.KeyTypeVersion, c.messageKey, c.ts, c.seq)
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
