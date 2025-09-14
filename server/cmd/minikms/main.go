package main

import (
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"progressdb/pkg/security"
)

type KeyMeta struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Wrapped   string    `json:"wrapped"` // base64
	ThreadID  string    `json:"thread_id,omitempty"`
}

func main() {
	var socketPath string
	var dataDir string
	flag.StringVar(&socketPath, "socket", "/tmp/progressdb-minikms.sock", "unix socket path")
	flag.StringVar(&dataDir, "data-dir", "./minikms-data", "directory to store key metadata")
	flag.Parse()

	// Ensure data dir
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Fatalf("failed mk data dir: %v", err)
	}

	// Load master key from env if present
	if mk := os.Getenv("PROGRESSDB_MINIKMS_MASTER_KEY"); mk != "" {
		if err := security.SetKeyHex(mk); err != nil {
			log.Fatalf("invalid master key: %v", err)
		}
		log.Printf("miniKMS: master key loaded from env")
	} else {
		// generate ephemeral master key
		b := make([]byte, 32)
		if _, err := crand.Read(b); err != nil {
			log.Fatalf("failed gen master key: %v", err)
		}
		security.SetKeyHex(fmt.Sprintf("%x", b))
		for i := range b {
			b[i] = 0
		}
		log.Printf("miniKMS: ephemeral master key generated")
	}

	// HTTP handlers
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	// No API-key fallback: only allow connections authenticated by peer UID.

	auditPath := filepath.Join(dataDir, "minikms-audit.log")
	if f, err := os.OpenFile(auditPath, os.O_CREATE, 0600); err == nil {
		f.Close()
	}

	// Parse allowed uids from env for peer-cred-based auth
	allowedUIDs := map[int]struct{}{}
	if v := os.Getenv("PROGRESSDB_MINIKMS_ALLOWED_UIDS"); v != "" {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if uid, err := strconv.Atoi(p); err == nil {
				allowedUIDs[uid] = struct{}{}
			}
		}
	}

	requireRole := func(role string, h func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Allow based on peer UID if present and allowed
			if v := r.Context().Value("peer_uid"); v != nil {
				if uid, ok := v.(int); ok {
					if _, allowed := allowedUIDs[uid]; allowed {
						h(w, r, fmt.Sprintf("peer_uid:%d", uid))
						return
					}
				}
			}
			// No fallback; if peer UID not allowed, unauthorized.
			http.Error(w, "unauthorized", 401)
			return
		}
	}

	writeAudit := func(ev map[string]interface{}) {
		b, _ := json.Marshal(ev)
		sig := ""
		if s, err := security.AuditSign(b); err == nil {
			sig = s
		}
		line := map[string]interface{}{"event": json.RawMessage(b), "sig": sig}
		if lb, err := json.Marshal(line); err == nil {
			f, _ := os.OpenFile(auditPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
			if f != nil {
				f.Write(append(lb, '\n'))
				f.Close()
			}
		}
	}

	mux.HandleFunc("/create_dek_for_thread", requireRole("encrypt", func(w http.ResponseWriter, r *http.Request, actor string) {
		var req struct {
			ThreadID string `json:"thread_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		id, wrapped, err := createAndPersistDEK(dataDir, req.ThreadID)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		resp := map[string]string{"key_id": id, "wrapped": base64.StdEncoding.EncodeToString(wrapped)}
		json.NewEncoder(w).Encode(resp)
		writeAudit(map[string]interface{}{"type": "create_dek", "actor": actor, "key_id": id, "thread_id": req.ThreadID})
	}))

	// KEK rotation endpoint (admin only). Request body: { "new_kek_hex": "..." }
	mux.HandleFunc("/rotate_kek", requireRole("admin", func(w http.ResponseWriter, r *http.Request, actor string) {
		var req struct {
			NewKEK string `json:"new_kek_hex"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		nk := strings.TrimSpace(req.NewKEK)
		if nk == "" {
			http.Error(w, "missing new_kek_hex", 400)
			return
		}
		// decode new KEK
		nb, err := hex.DecodeString(nk)
		if err != nil || len(nb) != 32 {
			http.Error(w, "invalid new_kek_hex", 400)
			return
		}

		// Backup current DEKs directory
		metaDir := filepath.Join(dataDir, "kms-deks")
		backupDir := filepath.Join(dataDir, "kms-deks-backup")
		os.MkdirAll(backupDir, 0700)

		files, _ := os.ReadDir(metaDir)
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			fname := f.Name()
			path := filepath.Join(metaDir, fname)
			b, err := os.ReadFile(path)
			if err != nil {
				http.Error(w, "read meta failed", 500)
				return
			}
			var m KeyMeta
			if err := json.Unmarshal(b, &m); err != nil {
				http.Error(w, "meta decode failed", 500)
				return
			}
			// decode old wrapped
			wrappedOld, err := base64.StdEncoding.DecodeString(m.Wrapped)
			if err != nil {
				http.Error(w, "wrapped decode failed", 500)
				return
			}
			// unwrap with current KEK (global)
			dek, err := security.Decrypt(wrappedOld)
			if err != nil {
				http.Error(w, "unwrap with old key failed", 500)
				return
			}
			// wrap with new KEK bytes
			newWrapped, err := security.EncryptWithKeyBytes(nb, dek)
			// zeroize dek
			for i := range dek {
				dek[i] = 0
			}
			if err != nil {
				http.Error(w, "wrap with new key failed", 500)
				return
			}
			// backup old meta
			_ = os.WriteFile(filepath.Join(backupDir, fname+"."+time.Now().Format("20060102T150405")), b, 0600)
			// update meta
			m.Wrapped = base64.StdEncoding.EncodeToString(newWrapped)
			m.Version = m.Version + 1
			nbjson, _ := json.Marshal(m)
			if err := os.WriteFile(path, nbjson, 0600); err != nil {
				http.Error(w, "write meta failed", 500)
				return
			}
		}

		// Finally set global KEK to new value so subsequent ops use it
		if err := security.SetKeyHex(nk); err != nil {
			http.Error(w, "set new kek failed", 500)
			return
		}

		writeAudit(map[string]interface{}{"type": "rotate_kek", "actor": actor, "time": time.Now().UTC().Format(time.RFC3339)})
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))

	mux.HandleFunc("/get_wrapped", requireRole("admin", func(w http.ResponseWriter, r *http.Request, actor string) {
		keyID := r.URL.Query().Get("key_id")
		if keyID == "" {
			http.Error(w, "missing key_id", 400)
			return
		}
		metaPath := filepath.Join(dataDir, "kms-deks", keyID+".json")
		b, err := os.ReadFile(metaPath)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		var m KeyMeta
		if err := json.Unmarshal(b, &m); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"wrapped": m.Wrapped})
		writeAudit(map[string]interface{}{"type": "get_wrapped", "actor": actor, "key_id": keyID})
	}))

	mux.HandleFunc("/encrypt", requireRole("encrypt", func(w http.ResponseWriter, r *http.Request, actor string) {
		var req struct {
			KeyID, Plaintext, AAD string `json:"key_id,plaintext,aad"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		wrapped := getWrappedFromDisk(dataDir, req.KeyID)
		if wrapped == nil {
			http.Error(w, "key not found", 404)
			return
		}
		dek, err := security.Decrypt(wrapped)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		pt, err := base64.StdEncoding.DecodeString(req.Plaintext)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		ct, err := security.EncryptWithRawKey(dek, pt)
		for i := range dek {
			dek[i] = 0
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"ciphertext": base64.StdEncoding.EncodeToString(ct)})
		writeAudit(map[string]interface{}{"type": "encrypt", "actor": actor, "key_id": req.KeyID})
	}))

	mux.HandleFunc("/decrypt", requireRole("decrypt", func(w http.ResponseWriter, r *http.Request, actor string) {
		var req struct {
			KeyID, Ciphertext, AAD string `json:"key_id,ciphertext,aad"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		wrapped := getWrappedFromDisk(dataDir, req.KeyID)
		if wrapped == nil {
			http.Error(w, "key not found", 404)
			return
		}
		dek, err := security.Decrypt(wrapped)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		ct, err := base64.StdEncoding.DecodeString(req.Ciphertext)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		pt, err := security.DecryptWithRawKey(dek, ct)
		for i := range dek {
			dek[i] = 0
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"plaintext": base64.StdEncoding.EncodeToString(pt)})
		writeAudit(map[string]interface{}{"type": "decrypt", "actor": actor, "key_id": req.KeyID})
	}))

	// remove old socket
	_ = os.Remove(socketPath)
	l, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Fatalf("listen socket: %v", err)
	}
	_ = os.Chmod(socketPath, 0600)
	srv := &http.Server{
		Handler: mux,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			uid := peerUIDForConn(c)
			return context.WithValue(ctx, "peer_uid", uid)
		},
	}
	log.Printf("miniKMS listening on %s, data dir %s", socketPath, dataDir)
	if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
	// shutdown
	srv.Shutdown(context.Background())
}

func createAndPersistDEK(dataDir, threadID string) (string, []byte, error) {
	// generate DEK
	dek := make([]byte, 32)
	if _, err := securityRandRead(dek); err != nil {
		return "", nil, err
	}
	wrapped, err := security.Encrypt(dek)
	for i := range dek {
		dek[i] = 0
	}
	if err != nil {
		return "", nil, err
	}
	id := fmt.Sprintf("k_%d", time.Now().UnixNano())
	meta := KeyMeta{ID: id, CreatedAt: time.Now().UTC(), Wrapped: base64.StdEncoding.EncodeToString(wrapped), ThreadID: threadID}
	metaDir := filepath.Join(dataDir, "kms-deks")
	os.MkdirAll(metaDir, 0700)
	mb, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(metaDir, id+".json"), mb, 0600); err != nil {
		return "", nil, err
	}
	return id, wrapped, nil
}

func getWrappedFromDisk(dataDir, keyID string) []byte {
	metaPath := filepath.Join(dataDir, "kms-deks", keyID+".json")
	b, err := os.ReadFile(metaPath)
	if err != nil {
		return nil
	}
	var m KeyMeta
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	wb, err := base64.StdEncoding.DecodeString(m.Wrapped)
	if err != nil {
		return nil
	}
	return wb
}

// (no-op)

// peerUIDForConn is implemented per-platform in peercred_*.go
