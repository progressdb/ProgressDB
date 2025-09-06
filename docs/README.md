ProgressDB — Very short overview
--------------------------------

What is ProgressDB?
- A small, fast message store that runs as an HTTP server and exposes a simple JSON API for threads, messages, versions, replies and reactions.

How you work with it
- Run the server (from the repo root):
  - `go run ./server/cmd/progressdb` or build and run the binary.
- Your apps talk to the server over HTTP — the server is the service, your app is the client.

Quick examples
- Create a message:
  - `curl -X POST http://localhost:8080/v1/messages -H "Content-Type: application/json" -d '{"thread":"t1","author":"alice","body":{"text":"hi"}}'`
- List messages:
  - `curl "http://localhost:8080/v1/messages?thread=t1&limit=10"`
- View API docs:
  - Open `http://localhost:8080/docs` for Swagger UI or `http://localhost:8080/openapi.yaml` for the spec.

Notes
- For production, set a persistent DB path (`--db`), enable TLS and add API keys.
- See `server/docs/usage.md` for more start/connect examples.
