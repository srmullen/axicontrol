---
name: axicontrol-pause-resume
description: Pause and resume a running plot via checkpoint files
lane: review
tags: ready-for-agent, user-management
created-at: "2026-08-03"
created-by: seanmullen
assigned-to: claude
depends-on: axicontrol-print-whole-job
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (pause/resume transitions) and [ADR-0008](../docs/adr/0008-checkpoint-file-persistence.md) (checkpoint files).

## What to build

Pause and resume a running Pass, end to end, on the real device.

- Pause: pod sends SIGINT to the running `axicli` subprocess; it finishes its current line segment, writes its checkpoint, exits. Pass → `paused`.
- Every `axicli` invocation for a Pass (including its first) uses `-o checkpoints/<pass-id>.svg` in the `FileStore`, so pausing always has a checkpoint to write to.
- Resume: pod re-invokes `axicli` with `res_plot`/`res_home` against that same checkpoint key. Pass → `running`.
- Checkpoint retention: delete `checkpoints/<pass-id>.svg` as soon as the Pass reaches a terminal state (`complete`/`failed`/`cancelled`); delete-if-exists, since a `failed` Pass may not have left one.
- Cancel: from `pending`/`paused`/`running`, Pass and Job → `cancelled` (terminal).
- Pause/resume/cancel are htmx-triggered POSTs from the Job status page ([ADR-0012](../docs/adr/0012-frontend-stack.md)) — no full page reload, state reflects via the same page's SSE-driven updates ([axicontrol-notifications](./axicontrol-notifications.md)).

## Acceptance criteria

- [x] Pausing a running Pass stops the physical plot cleanly (no mid-line jump) and the Job reflects `paused` — **not verifiable in this session**, no AxiDraw hardware or `axicli` present; SIGINT/checkpoint wiring verified correct (see Execution Report), needs a run against real hardware
- [x] Resuming continues the same physical plot from where it paused, not from the beginning — **hardware-unverifiable** for the same reason; `res_plot` invocation against the checkpoint file verified correct
- [x] A Pass can be paused and resumed more than once — verified (`TestJobCanBePausedAndResumedMoreThanOnce`)
- [x] The checkpoint file is gone from the `FileStore` once the Pass reaches `complete`, `failed`, or `cancelled` — verified for all three terminal states
- [x] Cancelling a paused Pass leaves no checkpoint behind and the Job is `cancelled` — verified

## Blocked by

- axicontrol-print-whole-job

## Execution Report

**Date:** 2026-08-06

Built pause/resume/cancel end to end per ADR-0002/ADR-0008, on top of `axicontrol-print-whole-job`'s Job/Pass execution engine (`internal/api/jobs.go`/`jobs_run.go`).

**Checkpoints in `FileStore`**: extended the `FileStore` interface with `LocalWritePath` (like `LocalPath`, but doesn't require the key to already exist, and creates its parent directory — for a `-o` output target that may not exist yet on a Pass's first-ever invocation) and taught `PVStore.path()` to accept the `checkpoints/<pass-id>.svg` namespace (previously any nested key was rejected) while still rejecting nesting beyond that one sanctioned prefix, `..`, and absolute paths. Every `axicli` invocation — fresh or resumed — now passes `-o checkpoints/<pass-id>.svg`, so a pause always has somewhere to write to.

**SIGINT via context cancellation**: `Server.runAxicli`'s signature grew a `context.Context` (touching its one production implementation, `sysinfo.go`'s call site, and every test fake). The production implementation uses `exec.CommandContext` with `cmd.Cancel` overridden to send `os.Interrupt` (SIGINT) instead of the default SIGKILL, and leaves `WaitDelay` unset so it waits indefinitely for `axicli` to exit on its own — pausing means finishing the current line segment and writing a checkpoint, not an instant kill. `Server.beginRun`/`endRun`/`requestInterrupt` (`jobs_run.go`) track the in-flight invocation's cancel func plus an "intent" (`paused` or `cancelled`) an HTTP handler sets when it interrupts it; `executePass` reads that intent back once the (now-exited) invocation returns, to distinguish an intentional interrupt from a genuine failure regardless of `axicli`'s own exit code on SIGINT (untestable without hardware, so the code doesn't depend on it).

**Resume**: a Pass whose prior status was `paused` re-invokes `axicli` against its own checkpoint file via `--mode res_plot` (never `res_home`, which just parks the carriage rather than resuming plotting) instead of the original source SVG, ignoring `layerNumber` — the checkpoint already encodes which layer(s) were mid-progress.

**New endpoints**: `POST /jobs/{id}/pause` (only from `printing`), `POST /jobs/{id}/resume` (only from `paused`, reusing `tryStartNextPass`'s device-claim guard the same way advance does), `POST /jobs/{id}/cancel` (from `queued`/`awaiting-next-pass`/`paused`/`printing`; a running Pass is interrupted the same way pause is, landing on `cancelled` instead once `axicli` exits, while a not-yet-running one is cancelled synchronously). `jobs.html` gained Pause/Resume/Cancel buttons gated on `.Status`, riding the same htmx polling (`hx-trigger="every 2s"` while `printing`) `axicontrol-print-whole-job` established rather than the SSE path ADR-0006/`axicontrol-notifications` will eventually add — that ticket is still `todo` and explicitly out of scope here, same call the prior ticket made for its own "queryable while it runs" mechanism.

**Race fix found in review**: `executePass` was unconditionally writing `running` after reading a Pass's prior status, which could silently clobber a concurrent synchronous cancel (submit a Job, cancel it in the same instant `executePass`'s goroutine starts) — the Job would show `cancelled` while `axicli` ran anyway. Fixed with `tryStartPassRun`, a conditional `UPDATE ... WHERE status = ?` guarded on the exact status just read; `handleCancelJob`'s synchronous branch got the mirror-image guard (`tryCancelIfNotRunning`, `UPDATE ... WHERE status IN ('pending','paused')`), falling back to `requestInterrupt` if it loses that race (the Pass started running — e.g. via a concurrent resume — in the meantime). Both guards are unit-tested directly.

Ran `/code-review` (Standards + Spec axes) against the diff before committing. Spec review confirmed the SSE-vs-polling call above, confirmed `res_plot`-only (never `res_home`) was correct, and caught the `executePass` race described above (fixed) plus thin coverage on checkpoint deletion for a genuine, non-interrupted failure (added `TestJobFailureDeletesCheckpoint`). Standards review flagged (and this fixed): duplicated id-parse-plus-load-active-pass preamble across five handlers (extracted `loadJobActionTarget`), duplicated "setPassStatus, log on failure" shape in `executePass` (extracted `setPassStatusLogged`), and `runPass`/`buildAxicliArgs` growing to 7 positional params including two adjacent same-typed strings/int64s with real transposition risk (grouped into `passRun`/`axicliTarget` structs). Left two flagged points as-is: `filestore.go`'s `checkpoints/` namespace special-case (ADR-0008 already explicitly settled checkpoints living inside the same `FileStore` abstraction rather than a separate store — not this ticket's call to relitigate) and Job-status-as-untyped-string Primitive Obsession (pre-existing convention from `axicontrol-print-whole-job`/`axicontrol-layers-mode`, not something to unilaterally change mid-ticket).

Verified: `go build`/`go vet`/`golangci-lint run`/`go test -race ./...` all clean, run five times to check for flakiness. Three pre-existing tests (`TestJobLayersModeHoldsDeviceClaimThroughAwaitingNextPass`, `TestJobSubmitAppliesPassOverride`, `TestJobLayersModeAdvanceRejectedBeforeAwaitingNextPass`) submitted or unblocked a Pass and returned without waiting for its background goroutine to finish — harmless before this ticket (that goroutine touched no disk), but `-race` turned it into a real `TempDir` cleanup flake (`unlinkat ... directory not empty`) once checkpoint file I/O gave the dangling goroutine actual disk work to race the test's own cleanup against. Fixed by waiting for each Pass's own terminal status, matching every other test's pattern, surfaced one at a time across repeated `-race` runs. New tests cover: pause → checkpoint written → resume → completes via `res_plot` against that same checkpoint (`TestJobPauseThenResumeCompletesFromCheckpoint`); repeated pause/resume cycles (`TestJobCanBePausedAndResumedMoreThanOnce`); pause/resume rejected outside `printing`/`paused`; cancel from `printing` (interrupts, lands `cancelled`, checkpoint gone, device freed), from `paused` (checkpoint gone), and from `awaiting-next-pass` (synchronous); cancel rejected once terminal; checkpoint deletion on `complete`/`failed`/`cancelled`; the two atomic race-guard methods directly. Actually reaching a real AxiDraw (clean mid-line pause, true resume continuity) is unverifiable here, same hardware caveat `axicontrol-print-whole-job`/`axicontrol-backend-skeleton` recorded.
