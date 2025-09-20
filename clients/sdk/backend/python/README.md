# ProgressDB Python SDK (backend)

Lightweight Python SDK for backend callers of ProgressDB. Designed for server-side use (holds backend/admin API keys).

Install (when published):

  pip install progressdb

Quickstart

```py
from progressdb import ProgressDBClient

client = ProgressDBClient(base_url='https://api.example.com', api_key='ADMIN_KEY')

# Sign a user id (backend-only)
sig = client.sign_user('user-123')

# Create a thread (provide author)
thread = client.create_thread({'title': 'General'}, author='service-account')

# Create a message (provide author)
msg = client.create_message({'thread': thread['id'], 'body': {'text': 'hello'}}, author='service-account')
```

Features

- `sign_user(user_id)` — calls `POST /v1/_sign` (backend-only)
- `admin_health()`, `admin_stats()` — admin endpoints
- Thread and message helpers: `list_threads(author, title=None, slug=None)`, `create_thread(thread, author)`, `create_message(msg, author)`, `delete_thread(id, author)`, etc.

Thread-scoped message APIs

- `list_thread_messages(thread_id, limit=None, author=None)` — GET `/v1/threads/{thread_id}/messages`
- `get_thread_message(thread_id, id, author=None)` — GET `/v1/threads/{thread_id}/messages/{id}`
- `update_thread_message(thread_id, id, msg, author=None)` — PUT `/v1/threads/{thread_id}/messages/{id}`
- `delete_thread_message(thread_id, id, author=None)` — DELETE `/v1/threads/{thread_id}/messages/{id}`

Versions & reactions (thread-scoped)

- `list_message_versions(thread_id, id, author=None)` — GET `/v1/threads/{thread_id}/messages/{id}/versions`
- `list_reactions(thread_id, id, author=None)` — GET `/v1/threads/{thread_id}/messages/{id}/reactions`
- `add_or_update_reaction(thread_id, id, input, author=None)` — POST `/v1/threads/{thread_id}/messages/{id}/reactions`
- `remove_reaction(thread_id, id, identity, author=None)` — DELETE `/v1/threads/{thread_id}/messages/{id}/reactions/{identity}`

Deprecated / removed

- The older message-level endpoints (e.g. `GET /v1/messages/{id}`, `PUT /v1/messages/{id}`, `DELETE /v1/messages/{id}`, message-level versions/reactions) have been removed in favor of the thread-scoped APIs above.

Note: `list_threads` accepts optional query filters `author`, `title`, and `slug`.
Backend callers should provide `author` (either via this parameter or the
`X-User-ID` header) when using backend/admin keys because the server requires an
author to be resolved for this endpoint.

## Thread helpers (notes)

 - `get_thread(id, author)` — retrieve thread metadata (title, slug, author, timestamps).
   Backend callers should provide `author` when using backend/admin keys (via this
   parameter or by setting `X-User-ID` header) because the server requires author resolution
   for this endpoint.
