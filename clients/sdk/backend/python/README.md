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

# Create a thread
thread = client.create_thread({'title': 'General'})

# Create a message
msg = client.create_message({'thread': thread['id'], 'body': {'text': 'hello'}})
```

Features

- `sign_user(user_id)` — calls `POST /v1/_sign` (backend-only)
- `admin_health()`, `admin_stats()` — admin endpoints
-- Thread and message helpers: `list_threads(author=None, title=None, slug=None)`, `create_thread`, `create_message`, `delete_thread`, etc.

Note: `list_threads` accepts optional query filters `author`, `title`, and `slug`.
Backend callers should provide `author` (either via this parameter or the
`X-User-ID` header) when using backend/admin keys because the server requires an
author to be resolved for this endpoint.

## Thread helpers (notes)

- `get_thread(id, author=None)` — retrieve thread metadata (title, slug, author, timestamps).
  Backend callers should provide `author` when using backend/admin keys (either via this
  parameter or by setting `X-User-ID` header) because the server requires author resolution
  for this endpoint.
