---
name: axicontrol-layer-labels
description: DiscoverLayers returns raw layer label text; print page shows a labeled layer list
lane: todo
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
depends-on: axicontrol-print-page-preview
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md) and the `CONTEXT.md` glossary (Layer entry).

## What to build

`svg.DiscoverLayers` currently parses the leading integer off each `<g inkscape:groupmode="layer">`'s `inkscape:label` and discards the rest, returning bare layer numbers only (e.g. `5`). Extend it to also return the distinct raw label strings that collapsed into each number (e.g. `5` → `["5-red", "5-outlines"]`), and surface that on the print page as a simple read-only labeled layer list (e.g. "5 — red, outlines").

- Change `DiscoverLayers`'s return shape to carry label text per layer number, not just the number. Existing callers (`layers`-mode Job creation) only need the numbers and are otherwise unaffected.
- Print page (from `axicontrol-print-page-preview`) shows a plain list of this upload's layers using the new labels, whenever the upload has ≥1 discovered layer. No interactivity required yet (no selection, no per-layer preview) — that's `axicontrol-single-layer-mode`.
- No layers → no list shown (consistent with the mode-selector hiding rule in `axicontrol-print-page-job-creation`).

## Acceptance criteria

- [ ] `DiscoverLayers` (or a new function) returns each distinct layer number together with the raw label string(s) that produced it
- [ ] The print page lists an upload's layers by number and label when ≥1 layer exists
- [ ] Nothing is shown when the upload has 0 layers
- [ ] Existing Layers-mode Job creation (numbers only) is unaffected by the return-shape change

## Blocked by

- axicontrol-print-page-preview
