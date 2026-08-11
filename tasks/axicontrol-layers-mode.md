---
name: axicontrol-layers-mode
description: Multi-layer plot jobs with per-layer Passes
lane: done
tags: ready-for-agent, user-management
created-at: "2026-08-03"
created-by: seanmullen
assigned-to: claude
depends-on: axicontrol-print-whole-job
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (Job/Pass model, `layers` mode).

## What to build

Extend Jobs to `layers` mode: one Pass per auto-discovered layer number, printed one at a time with an explicit user trigger between them (no auto-advance).

- Parse the uploaded SVG's layer names for AxiDraw's numeric-prefix convention (e.g. `1 black`, `2 red`) to auto-discover Passes at Job-submission time.
- Job status gains `awaiting-next-pass`: reached when a Pass completes and a next Pass exists.
- An explicit "advance to next Pass" action starts the next Pass's `axicli` invocation; nothing starts automatically. This is an htmx button on the Job status page ([ADR-0012](../docs/adr/0012-frontend-stack.md)).
- Job reaches `complete` only once its last Pass completes.
- Layers mode reuses whole-mode's status machinery, pause/resume, and config resolution unchanged per Pass.

## Acceptance criteria

- [x] Uploading a multi-layer SVG and submitting a `layers` Job creates one Pass per discovered layer, in layer order
- [x] After a Pass completes with a next Pass pending, the Job sits in `awaiting-next-pass` indefinitely until the user triggers the advance — it does not auto-start
- [x] Triggering advance starts the next layer's `axicli` invocation
- [x] A `layers` Job reaches `complete` only after its final Pass completes
- [ ] An individual Pass within a `layers` Job can be paused/resumed (reuses axicontrol-pause-resume) — **not implementable yet**: `axicontrol-pause-resume` (this ticket's sibling, not a dependency) is still `lane: todo` with no pause/resume machinery anywhere in the codebase. Confirmed the design doesn't block it: pause/resume, once built, operates on a Pass exactly as whole-mode's does, layers-mode or not.

## Blocked by

- axicontrol-print-whole-job

## Execution Report

**Date:** 2026-08-06

Built `layers` mode on top of whole-mode's Job/Pass machinery, per ADR-0002:

- **Layer discovery** (`internal/svg/layers.go`, `DiscoverLayers`): walks an SVG's `<g inkscape:groupmode="layer">` elements (recursively, so sublayers count too), extracts each `inkscape:label`'s leading integer per AxiDraw's numeric-prefix convention, and returns the distinct numbers sorted ascending — e.g. `"5-red"` and `"5-outlines"` both map to layer 5 and collapse to one entry, matching AxiDraw's own "one Pass plots everything with that number prefix" semantics (confirmed against https://axidraw.com/doc/cli_api/: `--mode layers --layer N`). A non-numbered or unlabeled layer is simply skipped, not an error; an SVG with no numbered layers at all returns an empty slice, which `handleCreateJob` turns into a validation error ("no numbered layers found in this SVG") rather than silently creating a layers Job with zero Passes.
- **Migration `0005_pass_layer_number`**: adds a nullable `passes.layer_number` — null for whole-mode's single Pass, set for each layers-mode Pass.
- **Job status generalized to N Passes** (`internal/api/jobs.go`): replaced the whole-mode-only "join Job to its single sequence_index-0 Pass" query with `passSummary`/`deriveJobStatus`/`activePass`: `deriveJobStatus` walks a Job's Passes in sequence order and derives `queued`/`printing`/`awaiting-next-pass`/`complete`/`failed` from the first not-yet-complete one (pending at index 0 = queued, pending after a completed Pass = awaiting-next-pass — this is what makes whole-mode's existing single-Pass behavior fall out as the N=1 case, unchanged); `activePass` picks whichever Pass a Job's status/output/retry/advance actions pertain to. `loadJobs` fetches all Jobs' cores and all Passes (grouped by job_id) in exactly two queries total rather than one round trip per Job.
- **Submission** (`handleCreateJob`): a new `mode` form field (`whole`/`layers`, default `whole`). Layers mode reads the file's already-sanitized SVG back out of the `FileStore`, discovers its layer numbers, and creates one Pass per number via `insertJobAndPasses` (`insertJobAndPass`'s generalization) — every Pass shares the same Preset + overrides picked at submission time, matching the ticket's "config resolution unchanged per Pass" rather than the (unrequested) alternative of a distinct config per layer. The first Pass starts immediately, same as whole-mode.
- **Advance** (`POST /jobs/{id}/advance`, `handleAdvanceJob`): valid only when the Job's derived status is `awaiting-next-pass`; starts the next (already-`pending`) Pass. An htmx button on the Job row (`templates/jobs.html`) appears exactly when the row's status is `awaiting-next-pass`, alongside a new "Pass N/M" column and a `mode` selector on the submission form.
- **`axicli` invocation** (`jobs_run.go`): `buildAxicliArgs` takes a `layerNumber *int` — `nil` keeps whole-mode's `--mode plot`; a value switches to `--mode layers --layer N`. `executePass` reads the Pass's `layer_number` and threads it through.

**Found and fixed during `/code-review` (Standards + Spec axes) before committing**, run against `git diff HEAD` since nothing was committed yet this session:

- **Real device-safety bug (Spec review)**: the original diff kept the existing single-run lock exactly per-Pass — released as soon as *any* Pass finished, including a layers Pass that left the Job merely `awaiting-next-pass` rather than `complete`. That meant an unrelated Job could be submitted and fully run on the AxiDraw while a layers Job's artwork was still mounted mid-sequence, waiting on the user's own advance click — the opposite of what ADR-0002's "one continuous device-occupying sequence" framing and the ticket's whole point (pen changes on the *same* mounted sheet) require. Fixed by splitting the single `printing bool` into `deviceClaimed` (held from a Job's first Pass to its terminal state, spanning `awaiting-next-pass` gaps) and `passRunning` (guards only against double-starting a Pass, e.g. a double-clicked advance) — `tryClaimDevice`/`tryStartNextPass`/`releaseDevice(jobDone bool)` in `jobs_run.go`. A failed Pass still fully releases the claim (matching whole-mode's existing retry-reclaims-it behavior; a failure isn't a "safely paused, leave it mounted" state). Added `TestJobLayersModeHoldsDeviceClaimThroughAwaitingNextPass` to cover it: submits a layers Job, waits for `awaiting-next-pass`, confirms an unrelated whole-mode submission is rejected ("already printing"), advances, waits for `complete`, then confirms the device is free again.
- **N+1 query pattern (Standards review)**: `loadJobs` originally called the single-job `loadJobRow` (2 queries) once per Job. Restructured into `loadAllPassSummariesByJob` (one query, all Passes grouped by `job_id`) plus one job-cores query, so the list view is 2 queries total regardless of Job count, matching the flat-JOIN convention every other list loader (`loadPresets`, `loadUploads`) in this codebase already uses.
- **Duplicated retry/advance tail (Standards review)**: extracted `startPassAndRenderRow` for the `go s.executePass(...)` → reload → render sequence both handlers shared.
- **Mysterious `[]*int{nil}` sentinel (Standards review)**: replaced with a named `passLayerNumbersFor(mode, layerNumbers) []*int` and dropped an unneeded loop-variable reshadow (`n := n`) left over from pre-Go-1.22 habit.
- Not changed: a Data Clumps note on `insertJobAndPasses`'s growing parameter list, and the "Pass N/M" column showing `1/1` on whole-mode rows too (a side effect of unifying the row view, not asked for but harmless) — both judgement calls the reviewer flagged as low-severity/speculative; left alone per this repo's own precedent in `axicontrol-print-whole-job`'s execution report ("worth doing only if this shape grows").

Verified: `go build`/`go vet`/`golangci-lint run`/`go test -race ./...` all clean. Also ran the real service locally via `curl` end-to-end (upload a two-layer SVG → preset → submit `layers` Job → saw `1/2` printing → `axicli` not installed in this dev environment, same caveat `axicontrol-print-whole-job` recorded, so it failed with the expected `exec: "axicli": executable file not found` → retried → confirmed a whole-mode Job still works unchanged (`1/1`) → confirmed submitting `layers` mode against a non-layered SVG is rejected inline). Actually reaching a real AxiDraw and exercising a genuine two-color plot is unverifiable here, same hardware caveat as the prior ticket.
