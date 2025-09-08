ProgressDB Usage Guide
======================

Getting started
---------------

Start the server
----------------

Run the server from the `server` directory using Go:

`go run ./cmd/progressdb`

You can override the listen address and database path with flags:

`go run ./cmd/progressdb --addr ":8080" --db ./data/pebble`

There is an `env.example` and a `config.yaml` in the `server` directory — copy or edit them if you need to supply environment variables or a config file.

Connecting as a client (the API is the client)
---------------------------------------------

ProgressDB exposes a simple HTTP JSON API; your applications can talk to the server directly. The server also serves OpenAPI and a Swagger UI at `/openapi.yaml` and `/docs` respectively.

Common endpoints:

- `POST /v1/messages` — add a message (JSON body)
- `GET  /v1/messages?thread=<id>&limit=<n>` — list messages in a thread

Example requests (replace `:8080` with your `--addr` if different):

Create a message:
`curl -X POST http://localhost:8080/v1/messages \
  -H "Content-Type: application/json" \
  -d '{"body": {"text":"Hello"}, "thread":"t1", "author":"alice"}'`

List messages in a thread:
`curl "http://localhost:8080/v1/messages?thread=t1&limit=10"`

Swagger UI
----------

Open the browser at `http://localhost:8080/docs` to view and exercise the API.

Notes
-----

- For production, set a persistent DB path via `--db` and secure the server with API keys or TLS.
- See `server/cmd/progressdb` for how flags and config are parsed.
