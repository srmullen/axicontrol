---
name: axicontrol-layers-pause-ui
description: Layers-mode awaiting-next-pass screen shows finished/next layer label, preview, and progress
lane: todo
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

- [ ] While a Layers-mode Job is `awaiting-next-pass`, the print page shows the label of the layer that just completed
- [ ] It also shows the label and a live preview of the next layer about to run
- [ ] It shows overall progress (e.g. "N of M done")
- [ ] Advancing still requires an explicit user action — no auto-advance
- [ ] `/jobs`'s existing generic Pass N/M display is unaffected (this is print-page-specific; `/jobs` remains a simpler cross-file history view)

## Blocked by

- axicontrol-print-page-job-creation
- axicontrol-layer-labels
- axicontrol-single-layer-mode
