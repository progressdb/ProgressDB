# Logging Configuration

This document shows the small, centralized logging options for the server and example commands.

Environment variables
- `PROGRESSDB_LOG_MODE`: `dev` (default) or `prod` — controls Zap config presets.
- `PROGRESSDB_LOG_SINK`: `stdout` (default) or `file:/path/to/file` — controls where logs are written.
- `PROGRESSDB_LOG_LEVEL`: `debug|info|warn|error` — optional override for log level.

Examples
Default (development Zap config to stdout):
```bash
PROGRESSDB_LOG_MODE=dev ./scripts/dev.sh --no-enc
```

Production-style JSON to stdout:
```bash
PROGRESSDB_LOG_MODE=prod PROGRESSDB_LOG_LEVEL=info ./scripts/dev.sh --no-enc
```

Write logs to a file:
```bash
PROGRESSDB_LOG_MODE=prod PROGRESSDB_LOG_SINK=file:/tmp/progressdb.log ./scripts/dev.sh --no-enc
```

Notes
- Logging is initialized centrally via `server/pkg/logger.Init()` so all logging calls (via `server/pkg/logger.Log`) inherit the configured sink/format.
- Request headers are logged in a compact single-line representation to avoid multi-line text output. Sensitive headers are redacted automatically.
