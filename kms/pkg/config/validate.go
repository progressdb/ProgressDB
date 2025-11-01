package config

import (
	"encoding/hex"
	"errors"
)

func ValidateMasterKey(masterKey string) error {
	if masterKey == "" {
		return errors.New("master key cannot be empty")
	}

	keyBytes, err := hex.DecodeString(masterKey)
	if err != nil {
		return errors.New("master key is not valid hex")
	}

	if err := validateKeyStrength(keyBytes); err != nil {
		return err
	}

	return nil
}

func validateKeyStrength(key []byte) error {
	if len(key) != 32 {
		return errors.New("master key must be exactly 32 bytes (256 bits)")
	}

	if isWeakKeyPattern(key) {
		return errors.New("master key contains weak or predictable patterns")
	}

	uniqueBytes := make(map[byte]bool)
	for _, b := range key {
		uniqueBytes[b] = true
	}

	if len(uniqueBytes) < 16 {
		return errors.New("master key has insufficient uniqueness")
	}

	return nil
}

func isWeakKeyPattern(key []byte) bool {
	allZeros := true
	for _, b := range key {
		if b != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		return true
	}

	allOnes := true
	for _, b := range key {
		if b != 0xFF {
			allOnes = false
			break
		}
	}
	if allOnes {
		return true
	}

	if len(key) > 1 {
		first := key[0]
		allSame := true
		for _, b := range key {
			if b != first {
				allSame = false
				break
			}
		}
		if allSame {
			return true
		}
	}

	sequential := true
	for i := 1; i < len(key); i++ {
		if key[i] != key[i-1]+1 {
			sequential = false
			break
		}
	}
	if sequential {
		return true
	}

	alternating := true
	for i := 2; i < len(key); i++ {
		if key[i] != key[i-2] {
			alternating = false
			break
		}
	}
	if alternating {
		return true
	}

	return false
}
