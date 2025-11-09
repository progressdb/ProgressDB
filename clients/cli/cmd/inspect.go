package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/cockroachdb/pebble"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(inspectCmd)
}

var inspectCmd = &cobra.Command{
	Use:   "inspect [database-path]",
	Short: "Inspect old database keys and patterns",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dbPath := args[0]
		inspectDatabase(dbPath)
	},
}

func inspectDatabase(dbPath string) {
	db, err := pebble.Open(dbPath, &pebble.Options{ReadOnly: true})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	iter, _ := db.NewIter(nil)
	defer iter.Close()

	count := 0
	threadKeys := 0
	messageKeys := 0
	versionKeys := 0
	otherKeys := 0

	fmt.Println("🔍 Inspecting database keys:")
	fmt.Println("=====================================")

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		count++

		switch {
		case strings.HasPrefix(key, "thread:"):
			threadKeys++
			if threadKeys <= 5 {
				fmt.Printf("Thread key %d: %s\n", threadKeys, key)
			}
		case strings.HasPrefix(key, "version:msg:"):
			versionKeys++
			if versionKeys <= 5 {
				fmt.Printf("Version key %d: %s\n", versionKeys, key)
			}
		case strings.Contains(key, ":msg:"):
			messageKeys++
			if messageKeys <= 5 {
				fmt.Printf("Message key %d: %s\n", messageKeys, key)
			}
		default:
			otherKeys++
			if otherKeys <= 5 {
				fmt.Printf("Other key %d: %s\n", otherKeys, key)
			}
		}
	}

	fmt.Printf("\n📊 Key Summary:\n")
	fmt.Printf("  Total keys: %d\n", count)
	fmt.Printf("  Thread keys: %d\n", threadKeys)
	fmt.Printf("  Message keys: %d\n", messageKeys)
	fmt.Printf("  Version keys: %d\n", versionKeys)
	fmt.Printf("  Other keys: %d\n", otherKeys)
}
