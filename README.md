# nomad-gateway

A lightweight HTTP API server that wraps the [Nomad](https://www.nomadproject.io/) API, designed to be called by an MCP server for AI-assisted homelab workload management. Primary use case: managing Minecraft servers for kids.

---

## Quick Start

### Prerequisites

- Go 1.24+
- A running Nomad cluster (1.6+ for HCL spec retrieval)
- A Nomad ACL token with the permissions described below

### Configuration

Copy `.env.example` to `.env` and fill in your values:

```bash
cp .env.example .env
```

| Variable | Required | Default | Description |
|---|---|---|---|
| `NOMAD_ADDR` | yes | — | Nomad server URL, e.g. `https://nomad.example.com` |
| `NOMAD_TOKEN` | yes | — | Nomad ACL token |
| `GATEWAY_API_KEY` | yes | — | Bearer token callers must present to this API |
| `PORT` | no | `8080` | Port to listen on |

In production, secrets are injected by Nomad's Vault Workload Identity — the app never talks to Vault directly.

### Run

```bash
go run ./cmd/server
```

### Docker

```bash
docker build -t nomad-gateway .
docker run --env-file .env -p 8080:8080 nomad-gateway
```

---

## Authentication

All endpoints except `GET /health` require:

```
Authorization: Bearer <GATEWAY_API_KEY>
```

The token is compared with `crypto/subtle.ConstantTimeCompare` to prevent timing attacks.

---

## Endpoints

### Health

#### `GET /health`

Unauthenticated. Verifies the service can reach Nomad. Used for container health checks.

```bash
curl http://localhost:8080/health
# 200: {"status":"ok","version":"v1.0.1"}
# 503: {"status":"unavailable","version":"v1.0.1"}
```

---

### Jobs

#### `GET /jobs`

Lists jobs. Optional `filter` param is a Nomad prefix match.

```bash
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs?filter=my-"
```

#### `GET /jobs/{jobID}`

Returns the full parsed job spec (JSON) for the given job.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service
```

#### `GET /jobs/{jobID}/spec`

Returns the original source (HCL or JSON) that was submitted when the job was last registered. Requires Nomad 1.6+.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service/spec
```

#### `POST /jobs`

Submits or updates a job. Body must be a raw HCL job spec. The original HCL is preserved in Nomad so it remains accessible via the UI and the `/spec` endpoint.

```bash
curl -X POST -H "Authorization: Bearer $KEY" \
  -H "Content-Type: text/plain" \
  --data-binary @my-job.hcl \
  http://localhost:8080/jobs
```

Returns the Nomad register response including `eval_id`.

#### `DELETE /jobs/{jobID}`

Stops (deregisters) a job. Add `?purge=true` to fully remove it from Nomad.

```bash
# Stop (keep job history)
curl -X DELETE -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service

# Stop and purge
curl -X DELETE -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service?purge=true"
```

Returns `{"eval_id": "..."}`.

#### `GET /jobs/{jobID}/health`

Blocks until all allocations for the job are running and `DeploymentStatus.Healthy == true`, or the timeout is reached.

| Param | Default | Description |
|---|---|---|
| `timeout` | `5m` | Any Go duration string: `30s`, `2m`, `5m`, `10m` |

```bash
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service/health?timeout=5m"
# 200: {"job_id":"my-service","healthy":true}
# 408: {"code":"timeout","message":"job did not become healthy within the timeout period"}
# 502: {"code":"nomad_error","message":"..."} — terminal alloc (failed/lost/complete)
```

> **Important**: This endpoint uses Nomad's `DeploymentStatus.Healthy` flag, which is only set during a deployment. In-place allocation restarts (`POST .../restart`) do **not** reset this flag, so the health endpoint will return `healthy: true` immediately after a restart. To verify a restarted task is actually working, poll its logs instead.

#### `GET /jobs/{jobID}/evaluations`

Returns all evaluations for the job, most recent first.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service/evaluations
```

#### `GET /jobs/{jobID}/versions`

Returns the full version history for the job.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service/versions
```

#### `POST /jobs/{jobID}/revert`

Reverts the job to a specific version. The `version` parameter is required.

```bash
curl -X POST -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service/revert?version=3"
```

Returns the Nomad register response including `eval_id`.

---

### Allocations

#### `GET /jobs/{jobID}/allocations`

Lists all allocations for the current version of the job.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service/allocations
```

#### `GET /jobs/{jobID}/allocations/{allocID}`

Returns full allocation details including task states and allocated ports.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service/allocations/abc123
```

The response includes `AllocatedResources.Shared.Ports` — an array of `{Label, Value, To, HostIP}` objects showing which host ports were assigned.

#### `POST /jobs/{jobID}/allocations/{allocID}/restart`

Restarts tasks within an allocation in-place (no reschedule, no new allocation ID).

| Param | Default | Description |
|---|---|---|
| `task` | (all tasks) | Restart only this task; omit to restart all tasks |

```bash
# Restart all tasks
curl -X POST -H "Authorization: Bearer $KEY" \
  http://localhost:8080/jobs/my-service/allocations/abc123/restart

# Restart a specific task
curl -X POST -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service/allocations/abc123/restart?task=server"
```

Returns `{"status":"restarted"}`.

> After restart, use the logs endpoint to confirm the task came back up — the health endpoint is not reliable for in-place restarts (see note above).

#### `GET /jobs/{jobID}/allocations/{allocID}/logs`

Retrieves task logs from the allocation.

| Param | Values | Default | Notes |
|---|---|---|---|
| `task` | task name | — | **Required** |
| `type` | `stdout`, `stderr` | `stdout` | |
| `origin` | `start`, `end` | `end` | Where to start reading |
| `limit` | bytes (integer) | `51200` | Max bytes to return; `0` = all logs |

```bash
# Last 50KB of stdout (default)
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service/allocations/abc123/logs?task=server"

# Last 100KB of stderr
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service/allocations/abc123/logs?task=server&type=stderr&limit=102400"

# All logs from the beginning
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service/allocations/abc123/logs?task=server&limit=0"

# First 10KB from the start
curl -H "Authorization: Bearer $KEY" \
  "http://localhost:8080/jobs/my-service/allocations/abc123/logs?task=server&origin=start&limit=10240"
```

Returns plain text (`text/plain; charset=utf-8`).

> `limit=0` forces `origin=start` regardless of what you pass — it means "return everything from the beginning."

---

### Node Pools

#### `GET /node-pools`

Lists all node pools defined in Nomad.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/node-pools
```

> Node pools must be explicitly created with `nomad node pool apply` before they appear here. The built-in `default` pool is always present.

#### `GET /node-pools/{poolName}/nodes`

Lists all nodes belonging to the given node pool.

```bash
curl -H "Authorization: Bearer $KEY" \
  http://localhost:8080/node-pools/default/nodes
```

---

## Error Responses

All errors return JSON:

```json
{"code": "machine_readable_code", "message": "Human readable description"}
```

| HTTP Status | Typical Cause |
|---|---|
| `400` | Missing or invalid query parameter |
| `401` | Missing or invalid `Authorization` header |
| `408` | Health watch timed out |
| `502` | Nomad returned an error or is unreachable |
| `503` | `/health` — cannot reach Nomad |

---

## Nomad ACL Policy

Apply the bundled policy:

```bash
nomad acl policy apply \
  -description "nomad-gateway access policy" \
  nomad-gateway \
  deploy/nomad-gateway.policy.hcl
```

Then generate and store the token:

```bash
nomad acl token create -name="nomad-gateway-token" -policy="nomad-gateway"
```

---

## Releases

This project uses [Semantic Versioning](https://semver.org/). Docker images are published to `ghcr.io/lobo235/nomad-gateway` on every push to `main` (`latest` + short SHA) and on every `v*` tag (`v1.0.1`, `v1.0`, `latest`).

The running version is always visible via `GET /health`.

---

## Development

```bash
# Run tests
go test ./...

# Run with live Nomad (requires .env)
go run ./cmd/server
```

Tests use `httptest.NewServer` to mock the Nomad HTTP API — no live Nomad cluster required for `go test`.
