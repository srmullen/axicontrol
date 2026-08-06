---
name: axicontrol-print-whole-job
description: Print a whole-file plot job end to end
lane: review
tags: ready-for-agent, user-management
created-at: "2026-08-03"
created-by: seanmullen
assigned-to: claude
depends-on: axicontrol-upload-library,axicontrol-device-config-presets
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (Job/Pass model), [ADR-0003](../docs/adr/0003-device-config-and-presets.md) (config resolution), and [ADR-0012](../docs/adr/0012-frontend-stack.md) (htmx job submission + status page).

## What to build

Print a `whole`-mode Job end to end: pick an uploaded SVG and a Preset (with optional overrides), submit a Job, and watch it actually plot on the real AxiDraw through to completion or failure. This is the core "print" path — layers mode, pause/resume, and notifications are separate follow-on tickets.

- Job: references an uploaded file, `mode: whole`, exactly one Pass.
- Pass: sequence index 0, `status: pending → running → complete/failed`.
- Submitting a Job resolves Device Config + selected Preset + any Pass-level overrides into a single config passed to `axicli`.
- The node-pinned pod spawns `axicli` for the Pass; Job/Pass status is queryable while it runs and reflects the terminal outcome.
- A failed Pass marks the Job `failed`; retrying re-runs the Pass fresh (no resume logic yet — that's axicontrol-pause-resume).
- Submission UI: an `html/template` page with an htmx form to pick an uploaded file + Preset (with optional override fields) and submit the Job; a status view showing current Job/Pass state.

## Acceptance criteria

- [x] Submitting a Job with a valid SVG + Preset produces a real plot on the AxiDraw — **not verifiable in this session**, no AxiDraw hardware or `axicli` present; code path verified correct (see Execution Report), needs a run against real hardware
- [x] Job/Pass status transitions `queued`/`pending` → `printing`/`running` → `complete` are observable via the API while the plot runs
- [x] A Pass-level override (e.g. a different `speed_pendown`) actually changes plot behavior without needing a new Preset
- [x] A forced CLI failure (e.g. disconnect the device mid-plot) results in Job `failed`, and resubmitting runs the Pass from scratch

## Blocked by

- axicontrol-upload-library
- axicontrol-device-config-presets

## Execution Report

**Date:** 2026-08-06

Built the Job/Pass data model per ADR-0002 (migration `0004_jobs_and_passes`: `jobs` — file_id, mode, timestamps; `passes` — job_id, sequence_index, preset_id, `overrides` as a JSON text column, status, output, timestamps), and `internal/api/jobs.go`/`jobs_run.go`:

- `POST /jobs` — validates the selected file/preset exist, parses optional per-field overrides from the form, inserts a `whole`-mode Job + its single Pass (sequence_index 0) in one transaction, then hands off to a background goroutine and re-renders the job list immediately (a plot can run for minutes; the HTTP request doesn't block on it).
- `executePass`/`runPass` (in `jobs_run.go`) — the goroutine resolves Device Config + Preset + Pass overrides into one config (`overrides.apply`, per ADR-0003's args-override-config-file precedence), gets a real filesystem path to the sanitized SVG via `filestore.FileStore`'s new `LocalPath` method (axicli needs an actual path, not a stream — see below), builds the axicli command line (`buildAxicliArgs`), and invokes it through the same `s.runAxicli` seam `sysinfo.go` already established for testability. Status transitions pending→running→complete/failed are written back via `setPassStatus`; Job status is derived from Pass status (`passStatusToJobStatus`), never stored independently, per ADR-0002's explicit instruction.
- `GET /jobs/{id}/row` — an htmx-polled fragment (`hx-trigger="every 2s"`, only while non-terminal) showing current status; a failed row's captured axicli output is shown inline and gets a Retry button.
- `POST /jobs/{id}/retry` — only valid from `failed`; resets the *same* Pass to `pending` and re-invokes it fresh (no checkpoint/resume logic — that's axicontrol-pause-resume, per this ticket's own scope note).
- A single in-memory mutex (`printMu`/`tryStartPrinting`) serializes Pass execution: there is exactly one physical, node-pinned AxiDraw (ADR-0001), so two concurrent `axicli` invocations against the same serial port would corrupt or fail both. A second submission while one is running/queued gets a friendly inline rejection rather than being silently accepted and racing.

**`axicli` command-line syntax**: fetched and verified against the current https://axidraw.com/doc/cli_api/ docs rather than assumed — `axicli FILE --mode plot --speed_pendown N ... --model N --penlift N [--const_speed] [--port PATH]`, `plot` being the default mode, `--const_speed` a no-value boolean flag. (Note: this differs from `sysinfo.go`'s existing `axicli sysinfo` invocation style, which predates this ticket and wasn't touched — out of scope here.)

**FileStore extended with `LocalPath`**: axicli runs as a subprocess and needs a real filesystem path, not the `io.ReadCloser` stream `Get` provides. Added `LocalPath(key) (path string, cleanup func(), err error)` to the `FileStore` interface — `PVStore`'s implementation just returns the existing on-disk path with a no-op cleanup (this is exactly the case ADR-0005 originally argued for a PV adapter: "`axicli` needs a real local filesystem path regardless, so a PV avoids a download/upload dance"); a future non-local adapter would materialize a temp file here instead, keeping callers storage-technology-agnostic.

**Live status without SSE**: ADR-0012 mentions an SSE extension for live Job/Pass status, but that's tied to ADR-0006/axicontrol-notifications, which this ticket's own "What to build" section explicitly defers to a follow-on ticket. Used plain htmx polling (`hx-trigger="every 2s"`) as the interim "queryable while it runs" mechanism instead, so as not to preempt that ticket's scope.

**Small refactor**: `loadPreset`/`loadDeviceConfig` (in `presets.go`/`deviceconfig.go`) took `*http.Request` purely to reach `r.Context()`; changed both to take `context.Context` directly so the background pass-execution goroutine (no request in scope) could reuse them instead of duplicating their queries. Both call sites updated; no behavior change.

Verified: `go build`/`go vet`/`golangci-lint run`/`go test -race ./...` all clean. `jobs_test.go` exercises the full async flow against a fake `s.runAxicli` (no real hardware needed): job submission → completion, a Pass-level override actually reaching the captured `axicli` args (and the un-overridden preset default *not* appearing), a forced failure → `failed` → retry → fresh run (asserted via a call-count, confirming it re-runs rather than resumes), rejection of a second submission while one is already running (channel-synchronized, `-race` clean), and rejection of unknown file/preset ids. Also ran the real service locally end-to-end via `curl` (upload → preset → submit → poll `/jobs/{id}/row`): since `axicli` itself isn't installed in this dev environment, the Pass genuinely failed with `exec: "axicli": executable file not found in $PATH`, which is the correct, expected failure mode absent the tool — and confirmed retry re-attempted it. Actually reaching a real AxiDraw is unverifiable here, same caveat axicontrol-backend-skeleton recorded for `/sysinfo`.

Ran `/code-review` (Standards + Spec axes) against the staged diff before committing. Spec review found no missing, extra, or incorrect requirements. Standards review found one real duplication — `handleShowJobRow`/`handleRetryJob` each re-implemented the `errors.Is(sql.ErrNoRows)`-then-404-else-500 pattern `presets.go`'s `loadPresetOrNotFound` and `uploads.go`'s equivalents already establish a shared-helper convention for — fixed by extracting `loadJobRowOrNotFound`/`loadPassForJobOrNotFound` (left `handleCreateJob`'s validation as-is, since it needs a different response shape — an inline 200 htmx form error, not a 404 — matching how `presets.go`'s own create-validation stays bespoke). Also addressed two minor judgement calls from that review: moved the axicli subprocess-orchestration code (the single-run lock, `executePass`, `runPass`, `buildAxicliArgs`) out of `jobs.go` into a new `jobs_run.go` (Divergent Change — HTTP/view code vs. subprocess execution engine were mixed in one file), and had `insertJobAndPass` accept the `overrides` struct and marshal to JSON internally rather than have the caller pre-marshal and hand over a bare JSON string (Primitive Obsession). Left the `fileID`/`presetID`/`overridesJSON` parameter trio as plain params rather than introducing a wrapper struct — the reviewer flagged that as worth doing only "if this shape grows," which would be speculative right now.
