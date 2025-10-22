package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	kmss "github.com/progressdb/kms/pkg/security"
	"github.com/progressdb/kms/pkg/store"
	"gopkg.in/yaml.v3"
)

// validateKeyStrength performs additional validation on master key strength
func validateKeyStrength(key []byte) error {
	if len(key) != 32 {
		return errors.New("master key must be exactly 32 bytes (256 bits)")
	}

	// Check for common weak patterns
	if isWeakKeyPattern(key) {
		return errors.New("master key contains weak or predictable patterns")
	}

	// Basic entropy check - ensure key is not too repetitive
	uniqueBytes := make(map[byte]bool)
	for _, b := range key {
		uniqueBytes[b] = true
	}

	// At least 16 unique bytes out of 32 (50% uniqueness)
	if len(uniqueBytes) < 16 {
		return errors.New("master key has insufficient uniqueness")
	}

	return nil
}

// isWeakKeyPattern checks for common weak key patterns
func isWeakKeyPattern(key []byte) bool {
	// Check for all zeros
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

	// Check for all 0xFF
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

	// Check for repeated single byte
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

	// Check for sequential patterns (0x00, 0x01, 0x02, ...)
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

	// Check for alternating patterns
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

// validateConfigPath performs basic validation on the config file path
func validateConfigPath(cfgPath string) error {
	if cfgPath == "" {
		return errors.New("config path cannot be empty")
	}

	// Check if file exists
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		return fmt.Errorf("config file does not exist: %s", cfgPath)
	}

	// Check file permissions (should not be world-readable)
	info, err := os.Stat(cfgPath)
	if err != nil {
		return fmt.Errorf("cannot stat config file: %w", err)
	}

	// Check if others have read permission
	if info.Mode().Perm()&0004 != 0 {
		log.Printf("WARNING: Config file %s is world-readable. Consider restricting permissions.", cfgPath)
	}

	return nil
}

// loadMasterKeyFromConfig loads and validates master key from YAML config
func loadMasterKeyFromConfig(cfgPath string) (string, error) {
	// Validate config file path and permissions
	if err := validateConfigPath(cfgPath); err != nil {
		return "", err
	}

	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", fmt.Errorf("failed to read config file: %w", err)
	}

	var config struct {
		MasterKeyHex string `yaml:"master_key_hex"`
		MasterKey    string `yaml:"master_key"`
	}

	if err := yaml.Unmarshal(b, &config); err != nil {
		return "", fmt.Errorf("failed to parse config YAML: %w", err)
	}

	masterKey := config.MasterKeyHex
	if masterKey == "" {
		masterKey = config.MasterKey
	}

	if masterKey == "" {
		return "", errors.New("no master_key or master_key_hex found in config")
	}

	// Validate hex format
	keyBytes, err := hex.DecodeString(masterKey)
	if err != nil {
		return "", fmt.Errorf("master key is not valid hex: %w", err)
	}

	// Additional key strength validation
	if err := validateKeyStrength(keyBytes); err != nil {
		return "", fmt.Errorf("master key failed strength validation: %w", err)
	}

	return masterKey, nil
}

func main() {
	var (
		endpoint = flag.String("endpoint", "127.0.0.1:6820", "HTTP endpoint address (host:port) or full URL")
		dataDir  = flag.String("data-dir", "./kms-data", "data directory")
		cfgPath  = flag.String("config", "", "optional config yaml")
	)
	flag.Parse()

	// load config if provided
	var masterHex string
	if *cfgPath != "" {
		loadedKey, err := loadMasterKeyFromConfig(*cfgPath)
		if err != nil {
			log.Fatalf("failed to load master key from config %s: %v", *cfgPath, err)
		}
		masterHex = loadedKey
	}

	var provider kmss.KMSProvider
	if masterHex != "" {
		p, errProv := kmss.NewHashicorpProviderFromHex(context.Background(), masterHex)
		if errProv != nil {
			log.Fatalf("failed to init provider: %v", errProv)
		}
		provider = p
	}

	st, errStore := store.New(*dataDir + "/kms.db")
	if errStore != nil {
		log.Fatalf("failed to open store: %v", errStore)
	}
	defer st.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})

	// register handlers
	mux.HandleFunc("/create_dek_for_thread", createDEKHandler(provider, st))
	mux.HandleFunc("/get_wrapped", getWrappedHandler(st))
	mux.HandleFunc("/encrypt", encryptHandler(provider, st))
	mux.HandleFunc("/decrypt", decryptHandler(provider, st))
	mux.HandleFunc("/rewrap", rewrapHandler(provider, st))

	// choose listener
	addr := *endpoint
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("listen tcp %s: %v", addr, err)
	}
	log.Printf("listening on %s", addr)

	srv := &http.Server{Handler: mux}
	if errServe := srv.Serve(ln); errServe != nil && errServe != http.ErrServerClosed {
		log.Fatalf("server failed: %v", errServe)
	}
}

func randRead(b []byte) (int, error) {
	return crand.Read(b)
}

func mustDecodeBase64(s string) []byte {
	if s == "" {
		return nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		return b
	}
	return []byte(s)
}

// helper encrypt/decrypt using raw DEK (nonce|ciphertext format)
func encryptWithRaw(dek, plaintext []byte) (string, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := randRead(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(append(nonce, ct...)), nil
}

func decryptWithRaw(dek []byte, b64 string) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := data[:ns]
	ct := data[ns:]
	return gcm.Open(nil, nonce, ct, nil)
}

// HTTP handlers (return handler funcs so we can capture dependencies)
func createDEKHandler(provider kmss.KMSProvider, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if provider == nil {
			http.Error(w, "no provider configured", http.StatusInternalServerError)
			return
		}
		var req struct {
			ThreadID string `json:"thread_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		kid, wrapped, kekID, kekVer, err := provider.CreateDEKForThread(req.ThreadID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		meta := map[string]string{"wrapped": base64.StdEncoding.EncodeToString(wrapped), "thread_id": req.ThreadID}
		mb, _ := json.Marshal(meta)
		_ = st.SaveKeyMeta(kid, mb)
		out := map[string]string{"key_id": kid, "wrapped": base64.StdEncoding.EncodeToString(wrapped), "kek_id": kekID, "kek_version": kekVer}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

func getWrappedHandler(st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keyID := r.URL.Query().Get("key_id")
		if keyID == "" {
			http.Error(w, "missing key_id", http.StatusBadRequest)
			return
		}
		mb, err := st.GetKeyMeta(keyID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var m map[string]string
		_ = json.Unmarshal(mb, &m)
		out := map[string]string{"wrapped": m["wrapped"]}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}
}

func encryptHandler(provider kmss.KMSProvider, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			KeyID     string `json:"key_id"`
			Plaintext string `json:"plaintext"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mb, err := st.GetKeyMeta(req.KeyID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var m map[string]string
		_ = json.Unmarshal(mb, &m)
		// Prefer provider-level operation by dekID. If provider cannot
		// operate (e.g. provider has no mapping), fall back to reading
		// the wrapped blob from store and unwrapping directly.
		if ct, kv, err := provider.EncryptWithDEK(req.KeyID, mustDecodeBase64(req.Plaintext), nil); err == nil {
			_ = json.NewEncoder(w).Encode(map[string]string{"ciphertext": base64.StdEncoding.EncodeToString(ct), "key_version": kv})
			return
		}
		wrappedB, _ := base64.StdEncoding.DecodeString(m["wrapped"])
		// try to call UnwrapDEK on provider if available
		if u, ok := provider.(interface{ UnwrapDEK([]byte) ([]byte, error) }); ok {
			dek, err := u.UnwrapDEK(wrappedB)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ct, err := encryptWithRaw(dek, mustDecodeBase64(req.Plaintext))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"ciphertext": ct})
			return
		}
		http.Error(w, "encryption not supported", http.StatusInternalServerError)
	}
}

func decryptHandler(provider kmss.KMSProvider, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			KeyID      string `json:"key_id"`
			Ciphertext string `json:"ciphertext"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mb, err := st.GetKeyMeta(req.KeyID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var m map[string]string
		_ = json.Unmarshal(mb, &m)
		// Prefer provider-level operation by dekID.
		if pt, err := provider.DecryptWithDEK(req.KeyID, mustDecodeBase64(req.Ciphertext), nil); err == nil {
			_ = json.NewEncoder(w).Encode(map[string]string{"plaintext": base64.StdEncoding.EncodeToString(pt)})
			return
		}
		wrappedB, _ := base64.StdEncoding.DecodeString(m["wrapped"])
		if u, ok := provider.(interface{ UnwrapDEK([]byte) ([]byte, error) }); ok {
			dek, err := u.UnwrapDEK(wrappedB)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			pt, err := decryptWithRaw(dek, req.Ciphertext)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"plaintext": base64.StdEncoding.EncodeToString(pt)})
			return
		}
		http.Error(w, "decryption not supported", http.StatusInternalServerError)
	}
}

func rewrapHandler(provider kmss.KMSProvider, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			KeyID     string `json:"key_id"`
			NewKEKHex string `json:"new_kek_hex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mb, err := st.GetKeyMeta(req.KeyID)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var m map[string]string
		_ = json.Unmarshal(mb, &m)
		newWrapped, newKekID, newKekVer, err := provider.RewrapDEKForThread(req.KeyID, req.NewKEKHex)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m["wrapped"] = base64.StdEncoding.EncodeToString(newWrapped)
		nb, _ := json.Marshal(m)
		_ = st.SaveKeyMeta(req.KeyID, nb)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "key_id": req.KeyID, "wrapped": base64.StdEncoding.EncodeToString(newWrapped), "kek_id": newKekID, "kek_version": newKekVer})
	}
}
