---
name: axicontrol-backend-skeleton
description: Device-pinned backend skeleton deployable to k8s with embedded SQLite
lane: todo
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0001](../docs/adr/0001-node-pinned-device-access.md) (device access), [ADR-0007](../docs/adr/0007-single-backend-service.md) (single backend service), [ADR-0009](../docs/adr/0009-embedded-sqlite-datastore.md) (embedded SQLite), and [ADR-0011](../docs/adr/0011-backend-stack.md) (Go/stdlib backend stack).

## What to build

The Go service skeleton and its container image — the platform every other axicontrol feature runs inside. No UI, no Job/Pass model yet: just a running, configurable, containerized service with durable local storage and one endpoint proving it can reach the device.

**Scope note**: the k8s deployment itself (node affinity, `hostPath` device mount, PersistentVolume) lives in a separate homelab repo per [ADR-0013](../docs/adr/0013-build-ci-and-publish.md) — this ticket's only obligation to that repo is producing a container image that reads its device path, data directory, and port from env vars rather than assuming any particular mount layout. Verifying actual pod scheduling/hostPath/PV behavior happens in that repo, not here.

- Go module with a stdlib `net/http` server; config loaded via plain `os.Getenv` (device path, SQLite path, data directory, port), all with sane local-dev defaults.
- Embedded SQLite via `modernc.org/sqlite` (pure Go, no cgo); schema created/evolved via `golang-migrate` migrations run on startup.
- `log/slog` for structured logging.
- A `sysinfo` endpoint that shells out to `axicli sysinfo` and returns the result, and a second endpoint that writes/reads a trivial row to confirm SQLite persists across a process restart against the same path.
- A multi-stage Dockerfile producing a `distroless/static` final image — this is the artifact the homelab repo deploys.

## Acceptance criteria

- [ ] Service starts with device path, SQLite path, and data directory supplied via env vars (not hardcoded)
- [ ] SQLite schema is created via `golang-migrate` on first boot; a written row survives a process restart against the same path
- [ ] The `sysinfo` endpoint successfully runs `axicli sysinfo` against a real AxiDraw and returns its output
- [ ] `docker build` produces a `distroless/static`-based image that runs the service with no other runtime dependencies present in the image
- [ ] `go vet`/`golangci-lint` and `go test` pass on a clean checkout (this ticket's baseline for [axicontrol-ci-pipeline](./axicontrol-ci-pipeline.md) to automate)

## Blocked by

None - can start immediately
