package security

import "crypto/rand"

// securityRandReadImpl reads cryptographically secure random bytes.
func securityRandReadImpl(b []byte) (int, error) { return rand.Read(b) }
