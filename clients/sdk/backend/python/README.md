# @progressdb/python

Python SDK for backend callers of ProgressDB. Uses backend API keys to call frontend routes without signature requirements.

## Installation

```bash
pip install progressdb
```

## Quick Start

```python
from progressdb import ProgressDB
db = ProgressDB({ 
    'baseUrl': 'https://api.example.com', 
    'apiKey': 'your-backend-key'
})

db.create_thread({'title': 'General'}, 'user-123')
```

## API

### Client
```python
ProgressDB(options: SDKOptionsType)
```

### Messages
- `list_thread_messages(thread_key, query, user_id)` - List messages in thread
- `create_thread_message(thread_key, message, user_id)` - Create message
- `get_thread_message(thread_key, message_key, user_id)` - Get message
- `update_thread_message(thread_key, message_key, message, user_id)` - Update message
- `delete_thread_message(thread_key, message_key, user_id)` - Delete message

### Threads
- `create_thread(thread, user_id)` - Create thread
- `list_threads(query, user_id)` - List threads
- `get_thread(thread_key, user_id)` - Get thread
- `update_thread(thread_key, thread, user_id)` - Update thread
- `delete_thread(thread_key, user_id)` - Delete thread

### Health
- `healthz()` - Basic health check
- `readyz()` - Readiness check with version

### Signature
- `sign_user(user_id)` - Generate signature for user