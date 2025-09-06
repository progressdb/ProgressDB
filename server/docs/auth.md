Perfect — let’s break down **Option C (HMAC, Novu-style)** with a concrete example.

---

## 🔑 Concept

Instead of issuing a long-lived JWT, your **backend signs each client identity** (e.g. `userId`, `subscriberId`, or `accountId`) with a secret key.

* The client **never knows the secret**.
* The client only knows the **signature** (hash).
* Every request contains both the ID and its hash.
* Your server verifies the hash before processing.

---

## 📝 Example Flow

### 1. Backend: Generate HMAC

Say your SDK wants to authenticate a user `1234`.

**Go code (backend)**:

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

var secret = []byte("SUPER_SECRET_KEY") // from env

// Generate a hash for a given userID
func signUser(userID string) string {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(userID))
	return hex.EncodeToString(h.Sum(nil))
}

func main() {
	userID := "1234"
	signature := signUser(userID)
	fmt.Println("userID:", userID)
	fmt.Println("signature:", signature)
}
```

This produces:

```txt
userID: 1234
signature: 5e3a1a... (HMAC-SHA256)
```

---

### 2. Backend → Client

When the user logs in, your backend sends:

```json
{
  "userId": "1234",
  "signature": "5e3a1a..."
}
```

---

### 3. Client SDK: Attach to API Calls

Now, every request from the SDK includes both:

```http
POST /messages
Authorization: PublishableKey xyz123
X-User-ID: 1234
X-User-Signature: 5e3a1a...
```

---

### 4. Server Middleware: Verify

On your server, before processing:

```go
func verify(userID, signature string) bool {
	expected := signUser(userID) // same function as before
	return hmac.Equal([]byte(expected), []byte(signature))
}
```

If `verify` passes → request is authentic.
If it fails → reject with `401 Unauthorized`.

---

## ⚖️ Pros & Cons

✅ Very simple (no token refresh).
✅ No risk of “forgotten 1-year JWTs.”
✅ Signature only works for that user.
❌ No session → each request must carry `id + signature`.
❌ If you need revocation/expiry, you’ll need extra checks (e.g. blocklist).

---

So it’s basically:

* **Client** = just passes `id + hash` from backend.
* **Backend** = recomputes hash with secret, compares.

---

Would you like me to write a **full mini working example** (Go server with one `/auth` route to generate HMAC + one protected `/messages` route that verifies it)?

Centralized configuration
-------------------------

This repository now centralizes runtime configuration into the `config` package
and exposes runtime values (backend/signing keys) via package getters.
that is populated on startup from the config file and environment variables. The
signing middleware and signing endpoint read signing and backend keys from this
canonical source of truth so that environment variables are parsed once at startup
and then used consistently across the server.

Practical notes:

- Configure backend API keys in your config file under `security.api_keys.backend` or
  via the environment variable `PROGRESSDB_API_BACKEND_KEYS` (comma separated).
- The signing keys used for verifying HMAC signatures are by default the same as
  the backend keys, but an explicit `AUTHOR_SIGNING_SECRET` env var will be included
  as a compatibility fallback.
