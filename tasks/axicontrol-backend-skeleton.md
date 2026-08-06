---
name: axicontrol-backend-skeleton
description: Device-pinned backend skeleton deployable to k8s with embedded SQLite
lane: review
tags: ready-for-agent, user-management
created-at: "2026-08-03"
created-by: seanmullen
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0001](../docs/adr/0001-node-pinned-device-access.md) (device access), [ADR-0007](../docs/adr/0007-single-backend-service.md) (single backend service), [ADR-0009](../docs/adr/0009-embedded-sqlite-datastore.md) (embedded SQLite), [ADR-0011](../docs/adr/0011-backend-stack.md) (Go/stdlib backend stack), and [ADR-0014](../docs/adr/0014-python-base-image-for-axicli.md) (Python-capable final image, superseding ADR-0013's distroless/static plan for the reason below).

## What to build

The Go service skeleton and its container image — the platform every other axicontrol feature runs inside. No UI, no Job/Pass model yet: just a running, configurable, containerized service with durable local storage and one endpoint proving it can reach the device.

**Scope note**: the k8s deployment itself (node affinity, `hostPath` device mount, PersistentVolume) lives in a separate homelab repo per [ADR-0013](../docs/adr/0013-build-ci-and-publish.md) — this ticket's only obligation to that repo is producing a container image that reads its device path, data directory, and port from env vars rather than assuming any particular mount layout. Verifying actual pod scheduling/hostPath/PV behavior happens in that repo, not here.

- Go module with a stdlib `net/http` server; config loaded via plain `os.Getenv` (device path, SQLite path, data directory, port), all with sane local-dev defaults.
- Embedded SQLite via `modernc.org/sqlite` (pure Go, no cgo); schema created/evolved via `golang-migrate` migrations run on startup.
- `log/slog` for structured logging.
- A `sysinfo` endpoint that shells out to `axicli sysinfo` and returns the result, and a second endpoint that writes/reads a trivial row to confirm SQLite persists across a process restart against the same path.
- A multi-stage Dockerfile: a builder stage compiling the static Go binary, and a Python-capable final stage (`python:3.12-slim` + `libusb` + `axicli` installed per its documented method) since `axicli` itself needs a Python runtime that `distroless/static` cannot provide — see [ADR-0014](../docs/adr/0014-python-base-image-for-axicli.md), which supersedes the `distroless/static` final-image plan in [ADR-0013](../docs/adr/0013-build-ci-and-publish.md). This is the artifact the homelab repo deploys.

## Acceptance criteria

- [x] Service starts with device path, SQLite path, and data directory supplied via env vars (not hardcoded)
- [x] SQLite schema is created via `golang-migrate` on first boot; a written row survives a process restart against the same path
- [ ] The `sysinfo` endpoint successfully runs `axicli sysinfo` against a real AxiDraw and returns its output — **not verifiable in this session**, no AxiDraw hardware or `axicli` present; needs a run against real hardware
- [x] `docker build` produces an image (`python:3.12-slim`-based per [ADR-0014](../docs/adr/0014-python-base-image-for-axicli.md)) that runs the service and successfully invokes `axicli` as a subprocess, with no dependencies beyond what `axicli` itself requires
- [x] `go vet`/`golangci-lint` and `go test` pass on a clean checkout (this ticket's baseline for [axicontrol-ci-pipeline](./axicontrol-ci-pipeline.md) to automate)

## Blocked by

None - can start immediately

## Execution Report

**Date:** 2026-08-03

Built the Go module (`github.com/srmullen/axicontrol`): `internal/config` (env-based config, table-driven tests), `internal/store` (embedded `modernc.org/sqlite` + `golang-migrate` via embedded `iofs` migrations, single-connection pool per ADR-0009's single-writer model), `internal/api` (stdlib `net/http`, `GET /sysinfo` with a seam over `exec.Command` for testing, `POST`/`GET /heartbeat` as the write/read persistence check), and `cmd/axicontrold` wiring it together with graceful shutdown and `slog`.

**Deviation from ADR-0013, recorded as [ADR-0014](../docs/adr/0014-python-base-image-for-axicli.md):** the ticket's original `distroless/static` final-image plan can't host `axicli` — it's a Python tool (bundling a proprietary `axidrawinternal` wheel) needing a Python interpreter, libc, and `libusb`, none of which exist in `distroless/static` (no libc at all). Switched the final stage to `python:3.12-slim` with `libusb-1.0-0` and `axicli` installed via its documented pip-from-zip method. Verified in this session: image builds, `axicli --version` runs inside the container, and `/sysinfo` correctly attempts the real subprocess call (fails with a device-not-found-style exit status absent hardware, not "executable not found").

Verified locally: config env-var loading, SQLite persistence across a simulated process restart (both via `go test` and by literally restarting the compiled binary against the same file), the heartbeat write/read cycle end-to-end via `curl` both on the host and inside the built container, and `go vet`/`golangci-lint run`/`go test ./...` all clean.

**Not verified — needs real hardware:** the `sysinfo` endpoint actually reaching an AxiDraw. Per this ticket's own scope note, hardware/pod-scheduling verification belongs to the homelab repo's deployment, not here.
