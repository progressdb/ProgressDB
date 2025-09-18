package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	kmss "github.com/progressdb/kms/pkg/security"
	"github.com/progressdb/kms/pkg/store"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		socket  = flag.String("socket", "/tmp/progressdb-kms.sock", "socket path (unix) or address")
		dataDir = flag.String("data-dir", "./kms-data", "data directory")
		cfgPath = flag.String("config", "", "optional config yaml")
	)
	flag.Parse()

	// load config if provided
	var masterHex string
	if *cfgPath != "" {
		if b, err := os.ReadFile(*cfgPath); err == nil {
			var m map[string]string
			_ = yaml.Unmarshal(b, &m)
			masterHex = m["master_key_hex"]
			if masterHex == "" {
				masterHex = m["master_key"]
			}
		}
	}

	var provider kmss.KMSProvider
	if masterHex != "" {
		p, err := kmss.NewHashicorpProviderFromHex(context.Background(), masterHex)
		if err != nil {
			log.Fatalf("failed to init provider: %v", err)
		}
		provider = p
	}

	st, err := store.New(*dataDir + "/kms.db")
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer st.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("ok"))
	})

	mux.HandleFunc("/create_dek_for_thread", func(w http.ResponseWriter, r *http.Request) {
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
		kid, wrapped, err := provider.CreateDEKForThread(req.ThreadID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		meta := map[string]string{"wrapped": base64.StdEncoding.EncodeToString(wrapped), "thread_id": req.ThreadID}
		mb, _ := json.Marshal(meta)
		_ = st.SaveKeyMeta(kid, mb)
		out := map[string]string{"key_id": kid, "wrapped": base64.StdEncoding.EncodeToString(wrapped), "kek_id": "", "kek_version": ""}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/get_wrapped", func(w http.ResponseWriter, r *http.Request) {
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
	})

	encryptWithRaw := func(dek, plaintext []byte) (string, error) {
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

	decryptWithRaw := func(dek []byte, b64 string) ([]byte, error) {
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

	mux.HandleFunc("/encrypt", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			KeyID, Plaintext string `json:"key_id" json:"plaintext"`
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
		wrappedB, _ := base64.StdEncoding.DecodeString(m["wrapped"])
		dek, err := provider.UnwrapDEK(wrappedB)
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
	})

	mux.HandleFunc("/decrypt", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			KeyID, Ciphertext string `json:"key_id" json:"ciphertext"`
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
		wrappedB, _ := base64.StdEncoding.DecodeString(m["wrapped"])
		dek, err := provider.UnwrapDEK(wrappedB)
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
	})

	mux.HandleFunc("/rewrap", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			KeyID, NewKEKHex string `json:"key_id" json:"new_kek_hex"`
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
		wrappedB, _ := base64.StdEncoding.DecodeString(m["wrapped"])
		dek, err := provider.UnwrapDEK(wrappedB)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newProv, err := kmss.NewHashicorpProviderFromHex(context.Background(), req.NewKEKHex)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newWrapped, err := newProv.WrapDEK(dek)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m["wrapped"] = base64.StdEncoding.EncodeToString(newWrapped)
		nb, _ := json.Marshal(m)
		_ = st.SaveKeyMeta(req.KeyID, nb)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "key_id": req.KeyID, "kek_id": "local"})
	})

	// choose listener
	addr := *socket
	var ln net.Listener
	var err error
	if strings.HasPrefix(addr, "unix://") {
		addr = strings.TrimPrefix(addr, "unix://")
	}
	if strings.HasPrefix(addr, "/") {
		// unix socket
		_ = os.Remove(addr)
		ln, err = net.Listen("unix", addr)
		if err != nil {
			log.Fatalf("listen unix %s: %v", addr, err)
		}
		// set perms
		_ = os.Chmod(addr, 0600)
		log.Printf("listening on unix socket %s", addr)
	} else {
		ln, err = net.Listen("tcp", addr)
		if err != nil {
			log.Fatalf("listen tcp %s: %v", addr, err)
		}
		log.Printf("listening on %s", addr)
	}

	srv := &http.Server{Handler: mux}
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server failed: %v", err)
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
