# nomad-gateway — Claude Code Project Guide

A Go HTTP API server that wraps the Nomad API for an MCP server to manage homelab workloads (primarily Minecraft servers).

---

## Build, Test, Run

```bash
# Build
~/bin/go/bin/go build ./...

# Run tests
~/bin/go/bin/go test ./...

# Run tests with verbose output
~/bin/go/bin/go test -v ./...

# Run the server (requires .env or env vars set)
~/bin/go/bin/go run ./cmd/server

# Build a binary
~/bin/go/bin/go build -o nomad-gateway ./cmd/server
```

> Go is installed at `~/bin/go/bin/go` (Go 1.26.1). It is also on `$PATH` via `.bashrc`.

---

## Project Layout

```
nomad-gateway/
├── Dockerfile
├── go.mod / go.sum
├── .env.example              # dev template — never commit real values
├── .gitignore
├── CLAUDE.md                 # this file
├── README.md                 # operator/user documentation
├── cmd/
│   └── server/
│       └── main.go           # entry point
├── deploy/
│   ├── nomad-gateway.hcl             # Nomad job spec
│   └── nomad-gateway.policy.hcl     # Nomad ACL policy
└── internal/
    ├── config/
    │   └── config.go         # ENV var loading & validation
    ├── nomad/
    │   ├── client.go         # Nomad API wrapper
    │   └── client_test.go    # unit tests (mock httptest server)
    └── api/
        ├── server.go         # HTTP mux + Run()
        ├── middleware.go     # Bearer token auth + request logging
        ├── handlers.go       # all route handlers
        ├── errors.go         # writeError / writeJSON helpers
        └── health.go         # GET /health (unauthenticated)
```

---

## Configuration

All config via ENV vars. Loaded from `.env` file in development (via `godotenv`; missing file is silently ignored).

| Var | Required | Default | Purpose |
|---|---|---|---|
| `NOMAD_ADDR` | yes | — | Nomad server URL (e.g. `https://nomad.example.com`) |
| `NOMAD_TOKEN` | yes | — | Nomad ACL token |
| `GATEWAY_API_KEY` | yes | — | Bearer token for callers of this API |
| `PORT` | no | `8080` | Listen port |

In production, secrets are injected by Nomad's Vault Workload Identity — the app itself never talks to Vault.

---

## Architecture

- **HTTP router**: Standard `net/http` with Go 1.22+ path parameters — no external router dependency.
- **Nomad client**: `github.com/hashicorp/nomad/api` official Go client.
- **Auth**: `Authorization: Bearer <token>` middleware using `crypto/subtle.ConstantTimeCompare`.
- **Logging**: `log/slog` with JSON handler at INFO level. Version logged on startup. Every request logged via `requestLogger` middleware (method, path, status, duration_ms, remote_addr).
- **Versioning**: SemVer. Version embedded at build time via `-ldflags "-X main.version=<ver>"`, defaults to `"dev"`. Exposed in `GET /health` response.
- **Graceful shutdown**: `signal.NotifyContext` on SIGINT/SIGTERM with 30s drain.
- **WriteTimeout**: `10m15s` — must exceed the maximum health-watch timeout (default 5m).

---

## API Routes

All routes except `/health` require `Authorization: Bearer <GATEWAY_API_KEY>`.

| Method | Path | Notes |
|---|---|---|
| `GET` | `/health` | Unauthenticated; verifies Nomad connectivity; returns `version` field |
| `GET` | `/jobs` | `?filter=<prefix>` optional |
| `GET` | `/jobs/{jobID}` | |
| `GET` | `/jobs/{jobID}/spec` | Returns original HCL/JSON submission (Nomad 1.6+) |
| `POST` | `/jobs` | Body: raw HCL; preserves original HCL in Nomad via `RegisterOpts` |
| `DELETE` | `/jobs/{jobID}` | `?purge=true\|false` (default false) |
| `GET` | `/jobs/{jobID}/health` | `?timeout=<go-duration>` (default 5m); blocks until healthy or timeout |
| `GET` | `/jobs/{jobID}/evaluations` | |
| `GET` | `/jobs/{jobID}/versions` | |
| `POST` | `/jobs/{jobID}/revert` | `?version=<N>` required |
| `GET` | `/jobs/{jobID}/allocations` | |
| `GET` | `/jobs/{jobID}/allocations/{allocID}` | Full alloc info including ports |
| `POST` | `/jobs/{jobID}/allocations/{allocID}/restart` | `?task=<name>` optional |
| `GET` | `/jobs/{jobID}/allocations/{allocID}/logs` | See log params below |
| `GET` | `/node-pools` | |
| `GET` | `/node-pools/{poolName}/nodes` | |

### Log endpoint params

| Param | Values | Default | Notes |
|---|---|---|---|
| `task` | task name | — | **Required** |
| `type` | `stdout`, `stderr` | `stdout` | |
| `origin` | `start`, `end` | `end` | |
| `limit` | bytes (int) | `51200` (50KB) | `0` = all logs (forces `origin=start`) |

---

## Testing Approach

Tests live in `internal/nomad/client_test.go`. Each test spins up an `httptest.NewServer` that mimics the Nomad HTTP API for the specific operation being tested.

Key patterns:
- Test client uses `healthPollInterval: 10 * time.Millisecond` for fast health-watch tests.
- The `GetAllocLogs` test mock returns `node.HTTPAddr = "127.0.0.1:1"` (unreachable) to force server proxy fallback in Nomad's `queryClientNode`.
- Nomad's `Deregister` API returns `{"EvalID": "..."}` JSON — not a bare string.
- Nomad's `Restart` and `Revert` operations use HTTP `PUT` (not `POST`).
- Port type in allocation responses is `[]api.PortMapping` (not `api.AllocatedPorts`).

When adding a new `nomad.Client` method, add a corresponding test that:
1. Registers the mock Nomad endpoint
2. Calls the client method
3. Asserts the return value and that the mock was actually hit

---

## Coding Conventions

- No external router, ORM, or framework dependencies — keep the dependency footprint minimal.
- Error responses always use `writeError(w, status, code, message)` with a machine-readable `code` string.
- Route handlers return `http.HandlerFunc` (not implement `http.Handler`) for consistency.
- `nomad.Client` methods wrap all Nomad API errors with `fmt.Errorf("context: %w", err)`.
- HCL submission: always use `RegisterOpts` with `Submission` set — `Register` without it loses the original source.

---

## Known Limitations

- **WatchJobHealth after in-place restart**: Nomad's `Allocations().Restart()` is an in-place restart and does **not** reset `DeploymentStatus.Healthy`. The health endpoint will return immediately with `healthy: true` after a restart because the deployment status is already confirmed from the previous deploy. Use log polling (GET `.../logs`) to confirm the restarted task is functioning, not the health endpoint.

- **Node pools must be pre-created**: `GET /node-pools` returns only pools that exist in Nomad. Create them with `nomad node pool apply <file.hcl>` before expecting them to appear.

- **Nomad 1.6+ required** for `GET /jobs/{jobID}/spec` (uses the Job Submissions API).

---

## Nomad ACL Policy

The policy file is at `deploy/nomad-gateway.policy.hcl`. Apply it with:

```bash
nomad acl policy apply -description "nomad-gateway access policy" nomad-gateway deploy/nomad-gateway.policy.hcl
```

Required capabilities:
- `list-jobs`, `read-job`, `submit-job` — job management
- `read-logs` — log retrieval
- `alloc-lifecycle` — allocation restart
- `agent { policy = "read" }` — health check (pings agent self endpoint)
- `node { policy = "read" }` — node pool listing

---

## Versioning & Releases

SemVer (`MAJOR.MINOR.PATCH`). To cut a release:

```bash
git tag v1.2.3 && git push origin v1.2.3
```

This triggers the Docker workflow, which builds and pushes:
- `ghcr.io/lobo235/nomad-gateway:v1.2.3`
- `ghcr.io/lobo235/nomad-gateway:v1.2`
- `ghcr.io/lobo235/nomad-gateway:latest`
- `ghcr.io/lobo235/nomad-gateway:<short-sha>`

The version is embedded in the binary via `-ldflags "-X main.version=v1.2.3"` and returned by `GET /health`.

---

## Docker

```bash
# Build (version defaults to "dev")
docker build -t nomad-gateway .

# Build with explicit version
docker build --build-arg VERSION=v1.2.3 -t nomad-gateway .

# Run
docker run --env-file .env -p 8080:8080 nomad-gateway
```

Multi-stage build: `golang:1.24-alpine` → `alpine:3.21`. Binary is statically compiled (`CGO_ENABLED=0`).
