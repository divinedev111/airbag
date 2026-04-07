[![CI](https://github.com/divinedev111/airbag/actions/workflows/ci.yml/badge.svg)](https://github.com/divinedev111/airbag/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/divinedev111/airbag)](https://goreportcard.com/report/github.com/divinedev111/airbag)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

# airbag

Self-hosted crash monitoring with structured reports. Lightweight Sentry alternative — single binary, SQLite storage, Go SDK included.

Collects crash reports from your applications, groups them into issues by fingerprint, and provides an API for viewing and managing incidents.

## Features

- **Crash ingestion** — POST errors from any language/framework via JSON API
- **Issue deduplication** — groups events by fingerprint (type + message + stack frame)
- **Issue lifecycle** — open, resolved, ignored. Resolved issues auto-reopen on new events
- **Go SDK** — drop-in error reporting with automatic stack trace capture
- **SQLite storage** — single file, WAL mode, zero setup
- **REST API** — list issues, view events, resolve/ignore, project stats

## Quick Start

### Run the server

```bash
go install github.com/divinedev111/airbag/cmd/airbag@latest
airbag -addr :8080
```

### Report an error

```bash
curl -X POST http://localhost:8080/api/ingest/my-app \
  -H "Content-Type: application/json" \
  -d '{
    "type": "error",
    "message": "connection refused: database not reachable",
    "stacktrace": "main.go:42 in connectDB()\nserver.go:15 in main()",
    "level": "error",
    "environment": "production",
    "release": "v1.2.3"
  }'
```

### Go SDK

```go
import "github.com/divinedev111/airbag/sdk/go"

client := airbag.New("http://localhost:8080", "my-app")
client.SetRelease("v1.2.3")
client.SetEnvironment("production")

// Report an error
client.CaptureError(err)

// Recover from panics
defer func() {
    if r := recover(); r != nil {
        client.CapturePanic(r)
    }
}()
```

## API

### Ingest (from your app)

```
POST /api/ingest/:project_id
```

```json
{
  "type": "error",
  "message": "null pointer dereference",
  "stacktrace": "main.go:42\n\truntime.gopanic()",
  "level": "fatal",
  "environment": "production",
  "release": "v1.0.0",
  "user_id": "user-123",
  "tags": {"service": "api", "region": "us-east-1"},
  "extra": {"request_id": "req-abc"}
}
```

### Dashboard API

```bash
# List issues for a project
GET /api/projects/:project/issues?status=open&limit=50

# Project stats (24h)
GET /api/projects/:project/stats

# Events for an issue
GET /api/issues/:id/events?limit=20

# Resolve an issue
POST /api/issues/:id/resolve

# Ignore an issue
POST /api/issues/:id/ignore
```

### Example: List open issues

```bash
curl http://localhost:8080/api/projects/my-app/issues?status=open
```

```json
[
  {
    "id": 1,
    "project_id": "my-app",
    "fingerprint": "a1b2c3d4e5f6g7h8",
    "type": "error",
    "message": "connection refused: database not reachable",
    "level": "error",
    "status": "open",
    "event_count": 47,
    "first_seen": "2026-04-01T10:00:00Z",
    "last_seen": "2026-04-03T14:32:00Z"
  }
]
```

## Architecture

```
cmd/airbag/
  main.go               Server entry point
internal/
  ingest/
    ingest.go           Crash report ingestion with fingerprinting
  store/
    store.go            SQLite storage (reports, issues, stats)
  api/
    api.go              Dashboard REST API
sdk/
  go/
    airbag.go           Go client SDK with stack capture
```

## Configuration

| Flag | Description | Default |
|------|-------------|---------|
| `-addr` | HTTP listen address | `:8080` |
| `-db` | SQLite database path | `airbag.db` |

## License

MIT
