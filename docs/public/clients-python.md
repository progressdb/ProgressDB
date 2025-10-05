---
section: clients
title: "Python Reference"
order: 2
visibility: public
---

# Python Backend SDK

Install (when published):

```bash
pip install progressdb
```

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

## API highlights

```py
# Signing (backend keys only)
def sign_user(user_id: str) -> dict:  # -> { 'userId': str, 'signature': str }

# Admin endpoints
def admin_health() -> dict
def admin_stats() -> dict

# Thread & message helpers
def list_threads(opts: dict = None) -> list
def get_thread(id: str, author: str = None) -> dict
def create_thread(thread: dict, author: str) -> dict
def delete_thread(id: str, author: str) -> None

# Thread-scoped message APIs
def list_thread_messages(thread_id: str, limit: int = None, author: str = None) -> list
def get_thread_message(thread_id: str, id: str, author: str = None) -> dict
def update_thread_message(thread_id: str, id: str, msg: dict, author: str = None) -> dict
def delete_thread_message(thread_id: str, id: str, author: str = None) -> None
```

Versions & reactions

- `list_message_versions(thread_id, id, author=None)` — GET `/v1/threads/{thread_id}/messages/{id}/versions`.
- Reactions: `list_reactions`, `add_or_update_reaction`, `remove_reaction` (thread-scoped).

Errors & retries

- Raises `ApiError` for non-2xx responses; the exception exposes response status and body.
- Configure retries for transient errors or wrap calls with your own retry logic.

Security

- Backend SDKs hold admin keys and must not be embedded in browser code.
- Use `sign_user` to produce `X-User-Signature` for frontend clients — backends should protect the signing endpoint.

Development

- Package layout is under `clients/sdk/backend/python/` in the repo. See the local `README.md` for tests and packaging notes.
