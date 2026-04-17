# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v1.3.3] - 2026-04-16

### Changed
- Consolidated `.gitea/workflows/docker.yml` into a single `ci.yml` with lint, test, build, and docker jobs. The docker push is now gated behind lint+test+build passing, so broken code cannot reach the registry or Nomad.
- Pin `golangci-lint` version in `.golangci-version` file (`v2.11.3`) as the single source of truth for local and CI. The Makefile's `lint` target installs the pinned version into `./bin/` via the official `install.sh` script, and the CI workflow calls `make lint` rather than duplicating install logic.
- Docker build now uses Buildx with registry layer caching (`:buildcache` tag, `mode=max`) for faster rebuilds.
- Makefile `test` and `cover` targets now pass `-race`.

## [v1.3.2] - 2026-04-16

### Fixed
- Restore hardcoded `REGISTRY: gitea.big.netlobo.com` env var in Gitea Docker workflow. The v1.3.1 scrub to `${{ secrets.REGISTRY_HOST }}` broke CI because that secret is not configured (only `REGISTRY_USER`, `REGISTRY_TOKEN`, `NOMAD_TOKEN`, `NOMAD_ADDR` are inherited at the org level). Per platform convention, the Gitea hostname is public and belongs in the workflow env block.

## [v1.3.1] - 2026-04-16

### Added
- `force_pull = true` in Nomad job spec Docker config
- `update` stanza in Nomad job spec with health checks and auto-revert

### Changed
- Dockerfile: run as non-root user (appuser, uid 1000) for container security
- Build output now goes to `bin/` directory instead of repo root
- `make clean` removes `bin/` directory instead of a bare binary
- `.gitignore` patterns scoped to repo root with leading `/` to prevent overbroad matching
- Registry hostname in deploy spec, CLAUDE.md, and Gitea workflow replaced with `gitea.example.com` placeholder / `${{ secrets.REGISTRY_HOST }}`

## [v1.3.0] - 2026-04-15

### Added
- `POST /jobs/plan` endpoint — dry-run a job spec to preview changes (diff, warnings, failed allocations) without registering

## [v1.2.5] - 2026-04-09

### Added
- Gitea Actions Docker workflow for CI/CD
- `make deploy` Makefile target
- Auto-deploy via Nomad Variables

### Changed
- Switch container registry to Gitea
- Update README version and add missing files to CLAUDE.md layout

## [v1.2.4] - 2026-03-24

### Added
- `grep` query parameter on log endpoint for server-side log filtering

### Changed
- Expanded API and Nomad client test coverage

## [v1.2.3] - 2026-03-24

### Fixed
- Replace private IP in test code with RFC 5737 documentation range address
- Update stale version in README health check example

## [v1.2.2] - 2026-03-24

### Changed
- Health check response includes descriptive status string ("all allocations running and healthy") instead of empty string

## [v1.2.1] - 2026-03-24

### Fixed
- Health check (`watch_job_health`) now skips terminal allocations (complete, failed, lost) instead of returning an error — fixes false failures on jobs with leftover allocations from previous deployments
- Correct Vault secret path in deploy spec to use `kv/data/nomad/default/nomad-gateway`

### Changed
- Docker build workflow resolves version from git tags for non-tag builds

## [v1.1.0] - 2026-03-21

### Added
- **`LOG_LEVEL` environment variable** — control log verbosity at runtime.
  Accepted values: `debug`, `info`, `warn`, `error`. Defaults to `info`.
  Invalid values are rejected at startup with a clear error message.
  The active level is included in the startup log line.

### Changed
- `nomadClient` interface extracted in `internal/api` — decouples the HTTP
  server from the concrete Nomad client implementation, enabling unit tests
  without a live Nomad cluster.
- Makefile added with standardised build commands: `make build`, `test`, `lint`,
  `cover`, `run`, `clean`, `hooks`.
- `.golangci.yml` (golangci-lint v2) with errcheck, staticcheck, gocyclo,
  misspell, revive, and goimports. All lint issues resolved.
- Pre-commit hook added at `.githooks/pre-commit` — runs lint and tests before
  every commit. Activate with `make hooks`.
- Doc comments added to all exported types and functions.

### Fixed
- `_ =` applied to `json.Encode` / `w.Write` calls to satisfy errcheck.

### Added
- `internal/api/server_test.go` with full handler coverage.
- `internal/config/config_test.go` covering all config scenarios.

## [v1.0.1] - 2026-03-20

### Added
- Version is now logged on startup at INFO level.

## [v1.0.0] - 2026-03-20

Initial production release.

### Features
- HTTP API wrapping the Nomad API for job and allocation management.
- Bearer token authentication via `GATEWAY_API_KEY`.
- Full job lifecycle: list, get, submit (HCL), stop/purge, revert, health watch.
- Allocation management: info, restart, log retrieval (stdout/stderr, offset, limit).
- Node pool and node listing.
- `GET /health` — unauthenticated; verifies Nomad connectivity; returns version.
- Structured JSON logging via `log/slog`.
- Graceful shutdown on SIGINT/SIGTERM with 30-second drain.
- SemVer embedded at build time; exposed in `/health` response.
- Multi-stage Docker image (`golang:1.24-alpine` → `alpine:3.21`).
- GitHub Actions workflow: builds and pushes to GHCR on `v*` tags with
  semver, floating `MAJOR.MINOR`, `latest`, and short-SHA tags.
- Nomad ACL policy and job spec in `deploy/`.
