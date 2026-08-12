---
name: axicontrol-layers-pause-ui
description: Layers-mode awaiting-next-pass screen shows finished/next layer label, preview, and progress
lane: done
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
depends-on: axicontrol-print-page-job-creation,axicontrol-layer-labels,axicontrol-single-layer-mode
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md), [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (`awaiting-next-pass`), and the `CONTEXT.md` glossary. This is the ticket most directly addressing "the way plotting layers works right now is confusing and not clear."

## What to build

Replace today's bare "Pass 2/4" column + generic "Advance to next pass" button with a clear picture of what's happening, on the print page, while a Layers-mode Job sits in `awaiting-next-pass`.

- Show which layer (number + label, from `axicontrol-layer-labels`) just finished.
- Show which layer is next, including a live preview of it, reusing the isolated-layer-preview endpoint built in `axicontrol-single-layer-mode`.
- Show overall progress (e.g. "2 of 4 done").
- The "Advance to next pass" action remains explicit/manual (no auto-advance — unchanged from ADR-0002's intent that layers mode usually means a manual pen swap); it just now sits alongside this context instead of a bare pass counter.

## Acceptance criteria

- [x] While a Layers-mode Job is `awaiting-next-pass`, the print page shows the label of the layer that just completed
- [x] It also shows the label and a live preview of the next layer about to run
- [x] It shows overall progress (e.g. "N of M done")
- [x] Advancing still requires an explicit user action — no auto-advance
- [x] `/jobs`'s existing generic Pass N/M display is unaffected (this is print-page-specific; `/jobs` remains a simpler cross-file history view)

## Blocked by

- axicontrol-print-page-job-creation
- axicontrol-layer-labels
- axicontrol-single-layer-mode

## Execution Report

Implemented in commit `2bc5d9f` on branch `work`.

- `passSummary` now reads `passes.layer_number` back (it existed in the schema already, just wasn't scanned), and `jobRowView` gains `LayerNumber` (active Pass's own layer) and `PrevLayerNumber` (the immediately preceding Pass's layer), computed in `buildJobRowView` (`internal/api/jobs.go`).
- New `printJobStatusView` (`internal/api/print.go`) wraps `jobRowView` with `FinishedLayer`/`NextLayer *layerView`, resolved via `buildPrintJobStatusView` only when a Job is `awaiting-next-pass` (every other status leaves them nil, and `jobRowView`/`job_row`/`/jobs` are untouched — no added cost to the general Job-loading path).
- `print_job_status` (`internal/api/templates/print.html`) now renders "N of M done", the finished layer's label, the next layer's label + live preview (`NextLayerPreviewSrc`, reusing the `/uploads/{id}/layers/{number}/content` isolated-layer endpoint from `axicontrol-single-layer-mode`), and an explicit "Advance to next pass" button, all gated on `Status == "awaiting-next-pass"`; every other status keeps the original bare "Pass N/M" line.
- Advance/retry/pause/resume/cancel are shared endpoints between `/jobs` and the print page; `renderJobActionResult` now picks `job_row` vs `print_job_status` based on htmx's own `HX-Target` request header, mirroring the existing GET-side `handleShowJobRow`/`handleShowJobPrintStatus` split — so the print page's own Advance button gets its status card back, not a `<tr>`.
- SSE OOB pushes (`events.go`) build the same `printJobStatusView` so the layers-pause context updates live via SSE too, not just polling.
- 7 new tests (2 unit tests on `buildJobRowView`'s new fields, 5 integration tests covering the print-page display, `/jobs` non-leakage, and the Advance-from-print-page fragment routing).
- Reviewed via `/code-review` (Standards + Spec sub-agents, both clean on spec compliance); fixed stale doc comments, a duplicated build/render sequence, an inconsistent nil-guard, and a duplicated layer-resolution block.
- `go build`, `go vet`, `gofmt -l`, `golangci-lint`, and the full test suite all pass.
