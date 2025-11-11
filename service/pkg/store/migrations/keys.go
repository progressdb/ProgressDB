package migrations

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/cockroachdb/pebble"
	"progressdb/pkg/state/logger"
)

// KeyMapping represents the mapping from old key to new key
type KeyMapping struct {
	OldKey    string `json:"old_key"`
	NewKey    string `json:"new_key"`
	ThreadID  string `json:"thread_id"`
	ThreadSeq int    `json:"thread_seq"`
	Timestamp int64  `json:"timestamp"`
	MsgSeq    int    `json:"msg_seq"`
	NewSeq    int    `json:"new_seq"`
	TSPadded  string `json:"ts_padded"`
	SeqPadded string `json:"seq_padded"`
}

// ParseThreadKey parses a thread meta key and extracts components
// Format: thread:thread-<thread_id>-<thread_seq>:meta
func ParseThreadKey(key string) (*KeyMapping, error) {
	pat := regexp.MustCompile(`^thread:(thread-(\d+)-(\d+)):meta$`)
	matches := pat.FindStringSubmatch(key)
	if matches == nil {
		return nil, fmt.Errorf("thread key format not recognized: %s", key)
	}

	threadID := matches[2]
	threadSeq, _ := strconv.Atoi(matches[3])
	threadTS, _ := strconv.ParseInt(threadID, 10, 64)

	return &KeyMapping{
		OldKey:    key,
		ThreadID:  threadID,
		ThreadSeq: threadSeq,
		Timestamp: threadTS,                         // Use thread timestamp as base
		MsgSeq:    0,                                // Threads don't have message sequences
		NewKey:    fmt.Sprintf("t:%020d", threadTS), // New format: t:<threadTS>
	}, nil
}

// ParseMessageKey parses a message key and extracts components
// Format: thread:thread-<thread_id>-<thread_seq>:msg:<timestamp>-<msg_seq>
func ParseMessageKey(key string) (*KeyMapping, error) {
	// Skip meta keys
	if strings.Contains(key, ":meta") {
		return nil, fmt.Errorf("meta key, skipping")
	}

	// Regex to extract components
	pat := regexp.MustCompile(`^thread:(thread-(\d+)-(\d+)):msg:(\d+)-(\d+)$`)
	matches := pat.FindStringSubmatch(key)
	if matches == nil {
		return nil, fmt.Errorf("message key format not recognized: %s", key)
	}

	threadID := matches[2]
	threadSeq, _ := strconv.Atoi(matches[3])
	timestamp, _ := strconv.ParseInt(matches[4], 10, 64)
	msgSeq, _ := strconv.Atoi(matches[5])

	return &KeyMapping{
		OldKey:    key,
		ThreadID:  threadID,
		ThreadSeq: threadSeq,
		Timestamp: timestamp,
		MsgSeq:    msgSeq,
	}, nil
}

// ConvertKeys converts old keys to new v2 format
func ConvertKeys(keys []string) ([]*KeyMapping, error) {
	var threadMappings []*KeyMapping
	var messageMappings []*KeyMapping

	// Parse all keys
	for _, key := range keys {
		// Try parsing as thread key first
		if strings.Contains(key, ":meta") {
			mapping, err := ParseThreadKey(key)
			if err != nil {
				logger.Debug("Skipping thread key", "key", key, "error", err)
				continue
			}
			threadMappings = append(threadMappings, mapping)
		} else {
			// Try parsing as message key
			mapping, err := ParseMessageKey(key)
			if err != nil {
				logger.Debug("Skipping message key", "key", key, "error", err)
				continue
			}
			messageMappings = append(messageMappings, mapping)
		}
	}

	// Group messages by thread ID
	threadGroups := make(map[string][]*KeyMapping)
	for _, mapping := range messageMappings {
		threadGroups[mapping.ThreadID] = append(threadGroups[mapping.ThreadID], mapping)
	}

	// Sort messages within each thread by timestamp, then original message seq
	for threadID, group := range threadGroups {
		sort.Slice(group, func(i, j int) bool {
			if group[i].Timestamp != group[j].Timestamp {
				return group[i].Timestamp < group[j].Timestamp
			}
			return group[i].MsgSeq < group[j].MsgSeq
		})

		// Renumber sequences zero-based within each thread
		for i, mapping := range group {
			mapping.NewSeq = i
			mapping.TSPadded = fmt.Sprintf("%020d", mapping.Timestamp)
			mapping.SeqPadded = fmt.Sprintf("%09d", i)

			// Build new key format: t:<thread_ts>:m:<message_ts>:<seq>
			mapping.NewKey = fmt.Sprintf("t:%s:m:%s:%s",
				mapping.TSPadded, // thread timestamp (from thread_id)
				mapping.TSPadded, // message timestamp
				mapping.SeqPadded)
		}

		threadGroups[threadID] = group
	}

	// Flatten messages back to single list
	var allMappings []*KeyMapping
	allMappings = append(allMappings, threadMappings...) // Add thread mappings first
	for _, group := range threadGroups {
		allMappings = append(allMappings, group...) // Add message mappings
	}

	return allMappings, nil
}

// WriteKeyMappings writes the key mappings to files
func WriteKeyMappings(mappings []*KeyMapping) error {
	// Write key mapping CSV
	mappingFile, err := os.Create("key_mapping_v2.csv")
	if err != nil {
		return fmt.Errorf("failed to create key mapping file: %w", err)
	}
	defer mappingFile.Close()

	// Write CSV header
	mappingFile.WriteString("old_key,new_key\n")

	// Write mappings
	for _, mapping := range mappings {
		line := fmt.Sprintf("%s,%s\n", mapping.OldKey, mapping.NewKey)
		if _, err := mappingFile.WriteString(line); err != nil {
			return fmt.Errorf("failed to write mapping: %w", err)
		}
	}

	// Write compact CSV for inspection
	compactFile, err := os.Create("migrated_compact_v2.csv")
	if err != nil {
		return fmt.Errorf("failed to create compact file: %w", err)
	}
	defer compactFile.Close()

	// Write CSV header
	compactFile.WriteString("thread_id,ts_padded,seq_padded,new_key\n")

	// Write compact data
	for _, mapping := range mappings {
		line := fmt.Sprintf("%s,%s,%s,%s\n",
			mapping.ThreadID,
			mapping.TSPadded,
			mapping.SeqPadded,
			mapping.NewKey)
		if _, err := compactFile.WriteString(line); err != nil {
			return fmt.Errorf("failed to write compact data: %w", err)
		}
	}

	logger.Info("Key mappings written",
		"count", len(mappings),
		"mapping_file", "key_mapping_v2.csv",
		"compact_file", "migrated_compact_v2.csv")

	return nil
}

// DumpAndConvertKeys reads all keys from database and converts them
func DumpAndConvertKeys(sourcePath string) error {
	// First dump all keys to a file
	if err := dumpKeysToFile(sourcePath); err != nil {
		return fmt.Errorf("failed to dump keys: %w", err)
	}

	// Read keys from file
	keys, err := readKeysFromFile("raw_keys.txt")
	if err != nil {
		return fmt.Errorf("failed to read keys: %w", err)
	}

	// Convert keys
	mappings, err := ConvertKeys(keys)
	if err != nil {
		return fmt.Errorf("failed to convert keys: %w", err)
	}

	// Write mappings
	if err := WriteKeyMappings(mappings); err != nil {
		return fmt.Errorf("failed to write mappings: %w", err)
	}

	return nil
}

// dumpKeysToFile dumps all database keys to a file
func dumpKeysToFile(sourcePath string) error {
	db, err := pebble.Open(sourcePath, &pebble.Options{
		ReadOnly: true,
	})
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer db.Close()

	keysFile, err := os.Create("raw_keys.txt")
	if err != nil {
		return fmt.Errorf("failed to create keys file: %w", err)
	}
	defer keysFile.Close()

	iter, _ := db.NewIter(nil)
	defer iter.Close()

	count := 0
	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		if _, err := keysFile.WriteString(key + "\n"); err != nil {
			return fmt.Errorf("failed to write key: %w", err)
		}
		count++
	}

	logger.Info("Keys dumped to file", "count", count, "file", "raw_keys.txt")
	return nil
}

// readKeysFromFile reads keys from a file
func readKeysFromFile(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	var keys []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			keys = append(keys, line)
		}
	}

	return keys, nil
}
