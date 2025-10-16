package store

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Key format constants and padding widths. Keep these in one place so
// formatting/parsing stays consistent across the codebase.
const (
	msgKeyFmt     = "thread:%s:msg:%s-%s"
	versionKeyFmt = "version:msg:%s:%s-%s"
	threadMetaFmt = "thread:%s:meta"

	tsPadWidth  = 20 // matches %020d used previously
	seqPadWidth = 6  // matches %06d used previously
)

var (
	// conservative ID validation: letters, digits, dot, underscore, dash
	// and a reasonable upper bound to protect DB key shapes.
	idRegexp = regexp.MustCompile(`^[A-Za-z0-9._-]{1,256}$`)
)

// ValidateThreadID ensures a thread id is safe to embed in keys.
func ValidateThreadID(id string) error {
	if id == "" {
		return errors.New("thread id empty")
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("invalid thread id: %q", id)
	}
	return nil
}

// ValidateMsgID ensures a message id is safe to embed in keys.
func ValidateMsgID(id string) error {
	if id == "" {
		return errors.New("msg id empty")
	}
	if !idRegexp.MatchString(id) {
		return fmt.Errorf("invalid msg id: %q", id)
	}
	return nil
}

// FormatTS returns a zero-padded timestamp string consistent with
// the rest of the codebase.
func FormatTS(ts int64) string {
	return fmt.Sprintf("%0*d", tsPadWidth, ts)
}

// FormatSeq returns a zero-padded sequence string.
func FormatSeq(seq uint64) string {
	return fmt.Sprintf("%0*d", seqPadWidth, seq)
}

// ParseTS parses a padded timestamp string previously formatted with FormatTS.
func ParseTS(s string) (int64, error) {
	if len(s) == 0 || len(s) > tsPadWidth {
		return 0, fmt.Errorf("ts length invalid: %s", s)
	}
	// allow legacy keys with fewer leading zeros by trimming left zeros
	trimmed := strings.TrimLeft(s, "0")
	if trimmed == "" {
		// string was all zeros
		return 0, nil
	}
	v, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse ts: %w", err)
	}
	return v, nil
}

// ParseSeq parses a padded sequence string previously formatted with FormatSeq.
func ParseSeq(s string) (uint64, error) {
	if len(s) == 0 || len(s) > seqPadWidth {
		return 0, fmt.Errorf("seq length invalid: %s", s)
	}
	trimmed := strings.TrimLeft(s, "0")
	if trimmed == "" {
		return 0, nil
	}
	v, err := strconv.ParseUint(trimmed, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse seq: %w", err)
	}
	return v, nil
}

// MsgKey builds a message key for a given thread, timestamp and sequence.
// It validates threadID and returns a formatted key or an error.
func MsgKey(threadID string, ts int64, seq uint64) (string, error) {
	if err := ValidateThreadID(threadID); err != nil {
		return "", err
	}
	tsStr := FormatTS(ts)
	seqStr := FormatSeq(seq)
	return fmt.Sprintf(msgKeyFmt, threadID, tsStr, seqStr), nil
}

// VersionKey builds a version index key for a message ID, timestamp and seq.
func VersionKey(msgID string, ts int64, seq uint64) (string, error) {
	if err := ValidateMsgID(msgID); err != nil {
		return "", err
	}
	tsStr := FormatTS(ts)
	seqStr := FormatSeq(seq)
	return fmt.Sprintf(versionKeyFmt, msgID, tsStr, seqStr), nil
}

// ThreadMetaKey returns the meta key for a thread id.
func ThreadMetaKey(threadID string) (string, error) {
	if err := ValidateThreadID(threadID); err != nil {
		return "", err
	}
	return fmt.Sprintf(threadMetaFmt, threadID), nil
}

// ParseMsgKey extracts components from a message key.
func ParseMsgKey(key string) (threadID string, ts int64, seq uint64, err error) {
	// expected shape: thread:<threadID>:msg:<ts>-<seq>
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 || parts[0] != "thread" || parts[2] != "msg" {
		err = fmt.Errorf("invalid msg key: %s", key)
		return
	}
	threadID = parts[1]
	if err = ValidateThreadID(threadID); err != nil {
		return
	}
	tail := parts[3]
	// split on last '-' to separate ts and seq
	i := strings.LastIndex(tail, "-")
	if i < 0 {
		err = fmt.Errorf("invalid msg key tail: %s", tail)
		return
	}
	tsPart := tail[:i]
	seqPart := tail[i+1:]
	ts, err = ParseTS(tsPart)
	if err != nil {
		return
	}
	seq, err = ParseSeq(seqPart)
	return
}

// ParseVersionKey extracts components from a version key.
func ParseVersionKey(key string) (msgID string, ts int64, seq uint64, err error) {
	// expected shape: version:msg:<msgID>:<ts>-<seq>
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 || parts[0] != "version" || parts[1] != "msg" {
		err = fmt.Errorf("invalid version key: %s", key)
		return
	}
	msgID = parts[2]
	if err = ValidateMsgID(msgID); err != nil {
		return
	}
	tail := parts[3]
	i := strings.LastIndex(tail, "-")
	if i < 0 {
		err = fmt.Errorf("invalid version key tail: %s", tail)
		return
	}
	tsPart := tail[:i]
	seqPart := tail[i+1:]
	ts, err = ParseTS(tsPart)
	if err != nil {
		return
	}
	seq, err = ParseSeq(seqPart)
	return
}

// MsgPrefix returns the prefix for message keys for a thread, e.g. "thread:<id>:msg:"
func MsgPrefix(threadID string) (string, error) {
	if err := ValidateThreadID(threadID); err != nil {
		return "", err
	}
	return fmt.Sprintf("thread:%s:msg:", threadID), nil
}

// ThreadPrefix returns the prefix for all keys under a thread, e.g. "thread:<id>:"
func ThreadPrefix(threadID string) (string, error) {
	if err := ValidateThreadID(threadID); err != nil {
		return "", err
	}
	return fmt.Sprintf("thread:%s:", threadID), nil
}