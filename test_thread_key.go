package main

import (
	"fmt"
	"regexp"
)

func main() {
	threadKeyRegexp := regexp.MustCompile(`^t:([A-Za-z0-9._-]{1,256})$`)
	key := "t:1761895711984857000"
	fmt.Printf("Key: %s\n", key)
	fmt.Printf("Matches: %v\n", threadKeyRegexp.MatchString(key))
	fmt.Printf("Key length: %d\n", len(key))
	fmt.Printf("Timestamp part length: %d\n", len("1761895711984857000"))
}
