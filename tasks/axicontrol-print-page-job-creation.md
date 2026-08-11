---
name: axicontrol-print-page-job-creation
description: Move job creation onto the print page; show this upload's own in-progress job inline; remove /jobs new-job form
lane: todo
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
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

- [ ] A Whole-mode Job can be submitted directly from an upload's print page
- [ ] A Layers-mode Job can be submitted directly from an upload's print page, when the upload has ≥1 discovered layer
- [ ] The mode selector is absent (not disabled — absent) when the upload has 0 discovered layers
- [ ] After submitting, the print page shows live status of that Job without navigating away
- [ ] Reloading the print page while a Job for this upload is in progress still shows its live status
- [ ] `/jobs` no longer offers a way to create a new Job, but still lists/manages existing ones
- [ ] Submitting a new Job while the device is already busy with another Upload's Job is still rejected (existing `deviceClaimed` behavior preserved)

## Blocked by

- axicontrol-print-page-preview
