package cmd

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/cockroachdb/pebble"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(debugCmd)
}

var debugCmd = &cobra.Command{
	Use:   "debug [database-path]",
	Short: "Debug database content with actual values",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dbPath := args[0]
		debugDatabase(dbPath)
	},
}

func debugDatabase(dbPath string) {
	db, err := pebble.Open(dbPath, &pebble.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	iter, _ := db.NewIter(nil)
	defer iter.Close()

	count := 0
	fmt.Println("🔍 Debugging database content:")
	fmt.Println("=====================================")

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		value := iter.Value()
		count++

		if count <= 10 {
			fmt.Printf("\nKey %d: %s\n", count, key)

			// Try to parse as JSON
			var jsonData interface{}
			if err := json.Unmarshal(value, &jsonData); err == nil {
				prettyJSON, _ := json.MarshalIndent(jsonData, "", "  ")
				fmt.Printf("Value: %s\n", string(prettyJSON))
			} else {
				fmt.Printf("Value (raw): %q\n", string(value))
			}
		}

		if count > 10 {
			break
		}
	}

	fmt.Printf("\nTotal keys processed: %d\n", count)
}
