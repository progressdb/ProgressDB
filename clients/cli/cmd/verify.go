package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/cockroachdb/pebble"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(verifyCmd)
}

var verifyCmd = &cobra.Command{
	Use:   "verify [database-path]",
	Short: "Verify Pebble database content",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dbPath := args[0]
		verifyPebbleDB(dbPath)
	},
}

func verifyPebbleDB(dbPath string) {
	db, err := pebble.Open(dbPath, &pebble.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	iter, _ := db.NewIter(nil)
	defer iter.Close()

	count := 0
	threadCount := 0
	messageCount := 0
	indexCount := 0
	relCount := 0

	fmt.Println("🔍 Verifying Pebble database content:")
	fmt.Println("=====================================")

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		count++

		switch {
		case key == "t:":
			// Skip empty key
		case len(key) > 2 && key[:2] == "t:" && key[2] != ':':
			threadCount++
			if count <= 5 {
				fmt.Printf("🧵 Thread: %s\n", key)
			}
		case key[:2] == "t:" && key[2] == ':':
			messageCount++
			if count <= 10 {
				fmt.Printf("💬 Message: %s\n", key)
			}
		case key[:4] == "idx:":
			indexCount++
			if count <= 15 {
				fmt.Printf("📇 Index: %s\n", key)
			}
		case key[:4] == "rel:":
			relCount++
			if count <= 20 {
				fmt.Printf("🔗 Relationship: %s\n", key)
			}
		}

		if count <= 20 {
			fmt.Printf("  Key %d: %s\n", count, key)
		}
	}

	fmt.Println("\n📊 Summary:")
	fmt.Printf("  Total keys: %d\n", count)
	fmt.Printf("  Threads: %d\n", threadCount)
	fmt.Printf("  Messages: %d\n", messageCount)
	fmt.Printf("  Indexes: %d\n", indexCount)
	fmt.Printf("  Relationships: %d\n", relCount)

	if count > 0 {
		fmt.Println("\n✅ Pebble migration completed successfully!")
	} else {
		fmt.Println("\n❌ No data found in database")
		os.Exit(1)
	}
}
