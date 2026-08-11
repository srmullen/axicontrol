---
name: axicontrol-print-page-job-creation
description: Move job creation onto the print page; show this upload's own in-progress job inline; remove /jobs new-job form
lane: done
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
assigned-to: claude
depends-on: axicontrol-print-page-preview
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md), [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (Job/Pass model), and the `CONTEXT.md` glossary.

## What to build

Move Job creation from `/jobs`'s generic form onto the per-upload print page (`axicontrol-print-page-preview`), scoped to that page's Upload, and show that upload's own in-progress Job inline once submitted.

- Print page gains a submission form for the existing **Whole** and **Layers** Plot Modes (Single Layer is a separate ticket), bound to this page's Upload — no more picking a file from a dropdown.
- Hide the mode selector entirely (submit as Whole with no choice shown) when the Upload has zero discovered layers (`svg.DiscoverLayers` returns empty) — don't show Layers as a disabled/dead option.
- If this Upload has a Job currently in progress (any non-terminal status), show its live status inline on the print page — not the file's full history, just the active Job, mirroring the live-update mechanism `/jobs` already uses (SSE/htmx polling).
- Remove the new-job form from `/jobs`; `/jobs` keeps its existing Retry/Advance/Pause/Resume/Cancel actions and becomes a pure cross-file history/monitoring view.
- Reuses the existing `deviceClaimed` guard (`internal/api/jobs_run.go`) unchanged — submitting while another Job is active is still rejected; this ticket doesn't need to explain *why* to the user (that's `axicontrol-device-busy-visibility`), just not break the existing block.

## Acceptance criteria

- [x] A Whole-mode Job can be submitted directly from an upload's print page
- [x] A Layers-mode Job can be submitted directly from an upload's print page, when the upload has ≥1 discovered layer
- [x] The mode selector is absent (not disabled — absent) when the upload has 0 discovered layers
- [x] After submitting, the print page shows live status of that Job without navigating away
- [x] Reloading the print page while a Job for this upload is in progress still shows its live status
- [x] `/jobs` no longer offers a way to create a new Job, but still lists/manages existing ones
- [x] Submitting a new Job while the device is already busy with another Upload's Job is still rejected (existing `deviceClaimed` behavior preserved)

## Blocked by

- axicontrol-print-page-preview

## Execution Report

**Date:** 2026-08-11

- New route `POST /uploads/{id}/jobs` (`internal/api/print.go`, `handleCreateJobForUpload`) creates a Job scoped to the path's Upload — no `file_id` form field for it to trust. The old `POST /jobs` route and its handler (`handleCreateJob`) were deleted outright, not left dead; `newJobFormView` and `rerenderJobsSection` went with them.
- Extracted `createJobForFile` (`internal/api/jobs.go`) from the old `handleCreateJob` body: validation, layer discovery, `tryClaimDevice`, `insertJobAndPasses`, and kicking off `executePass` — unchanged logic, just callable from the new upload-scoped handler instead of inline in one big handler.
- `print.html`'s `print_section` fragment carries the submission form (Preset + overrides, reused verbatim from the old `job_new_form`) plus a conditional Mode selector (`{{if .HasLayers}}`) — absent entirely, not disabled, when `svg.DiscoverLayers` finds nothing. A hidden `file_id` input stays in the form only because the Dry Run button still POSTs to the pre-existing, unmodified `/jobs/dry-run` endpoint, which expects it.
- `loadInProgressJobForFile`/`loadLatestJobForFile` (`internal/api/jobs.go`) derive a Upload's currently-active Job (if any) from its most recent Job's Passes — at most one can be non-terminal at a time, since `tryClaimDevice` never lets a second Job start against the same file while an earlier one is still active. Both the initial page load and the post-submission fragment reuse this, so a reload mid-Job and a fresh submission render identically.
- Live updates: `writeJobUpdateEvent` (`internal/api/events.go`) now renders *two* OOB-swap fragments per SSE `job-update` event — the existing `job_row` (`id="job-{ID}"`, unchanged, for `/jobs`) and a new `print_job_status` (`id="print-job-{ID}"`, for the print page) — both broadcast to every connected client; each page's own DOM only contains the id it cares about, so htmx's OOB matching silently drops the other. A polling fallback (`GET /jobs/{id}/print-status`) mirrors the existing `/jobs/{id}/row`.
- `/jobs` (`jobs.html`) lost its "New job" section, `job_new_form`, and the now-relocated `dry_run_result` template (moved to `print.html`, since it's only referenced there now) — it keeps the table and all of Retry/Advance/Pause/Resume/Cancel untouched.
- Deliberately **not** built: a global "device busy with another upload's Job" indicator on the print page. ADR-0017's broader narrative mentions this, but the ticket text explicitly defers it ("this ticket doesn't need to explain *why* ... that's `axicontrol-device-busy-visibility`") — the existing `deviceClaimed` guard still rejects the submission via the same `already printing` message, just without a proactive explanation.

**Verification:** `go build`, `go vet`, `golangci-lint run`, and `go test ./...` all clean. Ran the real service locally (`go run ./cmd/axicontrold`) and exercised it via curl: uploaded a layered and a plain SVG, confirmed the mode selector appears only for the layered one, submitted Whole- and Layers-mode Jobs directly from each print page (live "printing" status rendered inline in the submission response itself), reloaded a print page mid-Job and saw the same status persist, confirmed a terminal (failed, since no real `axicli` binary in this environment) Job drops off the print page but still appears in `/jobs`'s history with its Retry button, and confirmed `POST /jobs` now 405s while `/uploads/{id}/jobs` and `/uploads/{id}/print` 404 on an unknown upload id.

**Found and fixed during `/code-review` (Standards + Spec axes) before committing:** no code changes — Spec review found all 7 acceptance criteria implemented with matching tests, no scope creep, and no wrong implementations. Standards review raised two "hard violations," both investigated and not actioned: (1) the missing global busy indicator, which is explicitly out of scope per the ticket's own text (confirmed independently by the Spec review as *not* scope creep to add here); (2) a claimed-dead hidden `file_id` form field, which turned out to be load-bearing for the Dry Run button's request (confirmed by both reviews independently). Two minor judgement-call smells (a small `Status`-switch duplication between `Polling()`/`InProgress()`, and a three-primitive return shape on `createJobForFile`) were left as-is — mild, matching existing repo idioms, not worth the added abstraction.
