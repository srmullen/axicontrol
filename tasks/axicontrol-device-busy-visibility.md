---
name: axicontrol-device-busy-visibility
description: Any print page shows when the AxiDraw is busy with a different upload's job
lane: done
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
depends-on: axicontrol-print-page-job-creation
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md) and the `CONTEXT.md` glossary (Device-busy entry).

## What to build

Any print page — regardless of which Upload it belongs to — should proactively show when the AxiDraw is busy running a Job for a *different* Upload, instead of only rejecting a submission after the fact.

- The device-busy state already exists in-process (`deviceClaimed`, `internal/api/jobs_run.go`) but isn't exposed to any page. Expose which Upload/Job it's currently claimed by, and what it's doing (e.g. current layer/pass), to print pages other than the one that owns the running Job.
- On a print page whose own Upload isn't the busy one, show a banner/status reflecting this (e.g. "AxiDraw is busy: printing Layer 3/5 of sign.svg"), live-updating the same way the owning page's own status does.
- Blocking behavior itself does not change — new-Job submission is still rejected while `deviceClaimed` is true (existing behavior from `axicontrol-print-page-job-creation`); this ticket is purely about making that state visible ahead of a submit attempt, on every print page, not just the busy one.

## Acceptance criteria

- [x] Opening upload B's print page while upload A has a Job running shows that the device is busy and what it's doing (which file, which layer/pass if applicable)
- [x] That status updates live as the running Job progresses (e.g. layer/pass changes), without a manual reload
- [x] Once the running Job finishes, the busy banner clears from other print pages
- [x] Attempting to submit a new Job from a non-busy-owning print page while the device is busy is still rejected (unchanged from existing `deviceClaimed` behavior)

## Blocked by

- axicontrol-print-page-job-creation

## Execution Report

Implemented in commit `c482367` on branch `work`.

- Added `jobRowView.FileID` (threaded through `buildJobRowView` and its callers) and `loadLatestJob`/`loadBusyJobExcluding` (`internal/api/jobs.go`), which derive the device-busy Job from the jobs table by exploiting the invariant that at most one Job is ever in progress system-wide (enforced by the existing `tryClaimDevice` guard) — no new in-process state needed alongside `deviceClaimed`.
- `loadPrintPageView` now populates `printPageView.BusyJob` whenever the device-busy Job belongs to a *different* Upload than the page being viewed (`internal/api/print.go`).
- New `print_device_busy` template fragment (`internal/api/templates/print.html`) renders a "AxiDraw is busy: printing X — Pass N/M (status)" banner, polling a new `GET /uploads/{id}/busy-status` endpoint (`handleUploadBusyStatus`) every 2s — but only while there's a busy Job to show, matching `print_job_status`'s existing "poll only while active" discipline. The banner clears automatically once the busy Job reaches a terminal state.
- Submission-time blocking (`deviceBusyMessage`) is unchanged.
- 8 new tests in `internal/api/print_test.go` cover: banner shown for another Upload's running Job, banner absent for the owning Upload's own Job, banner absent when the device is free (and doesn't poll in that case), polling attributes present while busy, the busy-status fragment's own behavior (shows/excludes-self/clears-on-finish), and 404 handling.
- Reviewed via `/code-review` (Standards + Spec sub-agents); fixed the two substantive findings (duplicated row-loading code between `loadLatestJob`/`loadLatestJobForFile`, extracted into shared `loadJobRowByQuery`; and the banner's polling was gated to match `print_job_status`'s pattern rather than polling unconditionally forever).
- `go build`, `go vet`, `gofmt -l`, and the full test suite all pass.
