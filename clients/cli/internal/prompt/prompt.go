package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"progressdb/clients/cli/config"
)

// FillMissing prompts the user for missing configuration values
func FillMissing(cfg *config.Config) error {
	reader := bufio.NewReader(os.Stdin)

	if cfg.OldEncryptionKey == "" {
		key, err := promptForEncryptionKey(reader)
		if err != nil {
			return fmt.Errorf("failed to get encryption key: %w", err)
		}
		cfg.OldEncryptionKey = key
	}

	if cfg.FromDatabase == "" {
		path, err := promptForPath(reader, "Source database path", true)
		if err != nil {
			return fmt.Errorf("failed to get source database path: %w", err)
		}
		cfg.FromDatabase = path
	}

	if cfg.ToDatabase == "" {
		path, err := promptForPath(reader, "Target database path", false)
		if err != nil {
			return fmt.Errorf("failed to get target database path: %w", err)
		}
		cfg.ToDatabase = path
	}

	return nil
}

// promptForEncryptionKey prompts for the encryption key with masked input
func promptForEncryptionKey(reader *bufio.Reader) (string, error) {
	for {
		fmt.Print("Enter old encryption key (hex, 32 bytes): ")

		// Use term.ReadPassword for masked input
		keyBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			// Fallback to regular input if term.ReadPassword fails
			line, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			keyBytes = []byte(strings.TrimSpace(line))
		} else {
			// Print newline after password input
			fmt.Println()
		}

		key := strings.TrimSpace(string(keyBytes))
		if key == "" {
			fmt.Println("Encryption key cannot be empty.")
			continue
		}

		// Validate the key
		if err := config.ValidateEncryptionKey(key); err != nil {
			fmt.Printf("Invalid encryption key: %v\n", err)
			continue
		}

		// Confirm the key
		fmt.Print("Confirm encryption key: ")
		confirmBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			line, err := reader.ReadString('\n')
			if err != nil {
				return "", err
			}
			confirmBytes = []byte(strings.TrimSpace(line))
		} else {
			fmt.Println()
		}

		confirm := strings.TrimSpace(string(confirmBytes))
		if key != confirm {
			fmt.Println("Encryption keys do not match. Please try again.")
			continue
		}

		return key, nil
	}
}

// promptForPath prompts for a database path
func promptForPath(reader *bufio.Reader, label string, mustExist bool) (string, error) {
	for {
		fmt.Printf("%s: ", label)
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}

		path := strings.TrimSpace(line)
		if path == "" {
			fmt.Printf("%s cannot be empty.\n", label)
			continue
		}

		// Check if path exists (if required)
		if mustExist {
			if _, err := os.Stat(path); os.IsNotExist(err) {
				fmt.Printf("Path does not exist: %s\n", path)
				continue
			}
		}

		return path, nil
	}
}

// Confirm prompts for a yes/no confirmation
func Confirm(message string) bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [y/N]: ", message)
		line, err := reader.ReadString('\n')
		if err != nil {
			return false
		}

		response := strings.TrimSpace(strings.ToLower(line))
		switch response {
		case "y", "yes":
			return true
		case "n", "no", "":
			return false
		default:
			fmt.Println("Please enter 'y' or 'n'.")
		}
	}
}
