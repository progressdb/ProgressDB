package main

import (
	"fmt"
	"log"
	"os"

	"github.com/cockroachdb/pebble"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("Usage: go run dump_keys.go <db_path>")
	}

	db, err := pebble.Open(os.Args[1], &pebble.Options{})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	iter, _ := db.NewIter(nil)
	defer iter.Close()

	count := 0
	relUCount := 0
	relTCount := 0
	idxCount := 0

	for iter.First(); iter.Valid(); iter.Next() {
		key := string(iter.Key())
		count++

		if len(key) > 4 {
			switch key[:4] {
			case "rel:":
				if len(key) > 6 && key[4:6] == "u:" {
					relUCount++
				} else if len(key) > 6 && key[4:6] == "t:" {
					relTCount++
				}
			case "idx:":
				idxCount++
			}
		}

		if count <= 20 { // Show first 20 keys
			fmt.Printf("Key %d: %s\n", count, key)
		}
	}

	fmt.Printf("\nSummary:\n")
	fmt.Printf("Total keys: %d\n", count)
	fmt.Printf("rel:u:* (User->Thread): %d\n", relUCount)
	fmt.Printf("rel:t:* (Thread->User): %d\n", relTCount)
	fmt.Printf("idx:* (Indexes): %d\n", idxCount)
}
