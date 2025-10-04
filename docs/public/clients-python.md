---
section: clients
title: "Python Reference"
order: 2
visibility: public
---

# Python Backend SDK

Install the SDK (when published):

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

Key features

- `sign_user(user_id)` — obtain a signature for frontend usage
- `admin_health()`, `admin_stats()` — admin endpoints
- Thread & message helpers: `list_threads`, `create_thread`, `create_message`, `list_thread_messages`

Error handling

- The Python SDK raises `ApiError` for non-2xx responses and surfaces the
  status and response body for debugging.
- Configure retries for transient failures; the SDK supports a retry option
  or you can wrap calls with your own retry logic.

Development notes

- Python package layout lives under `clients/sdk/backend/python/` in the
  repository; build artifacts are in `dist/` when published. See their
  `README.md` for packaging and testing instructions.
