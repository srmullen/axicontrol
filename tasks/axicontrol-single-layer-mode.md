---
name: axicontrol-single-layer-mode
description: 'Single Layer plot mode: pick one labeled layer, preview it live, submit a one-Pass job for it alone'
lane: todo
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
depends-on: axicontrol-print-page-job-creation,axicontrol-layer-labels
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md), [ADR-0010](../docs/adr/0010-svg-sanitization-and-img-preview.md) (`<img>`-only preview constraint), and the `CONTEXT.md` glossary (Plot Mode entry — Single Layer is distinct from the existing sequential Layers mode: one Pass, one chosen Layer, no sequencing, no other Layers touched).

## What to build

Add **Single Layer** as a third Plot Mode alongside the existing Whole and Layers: the operator picks exactly one layer (by its labeled entry from `axicontrol-layer-labels`) from a dropdown, sees a live preview of just that layer, and submitting runs a single Pass against only that layer.

- New backend endpoint that renders an isolated single Layer as its own sanitized SVG document (hide/strip all other layer groups), served via `<img>` only — do not inline-embed or CSS-toggle visibility on the client, per ADR-0010's script-blocking rationale.
- Print page's Plot Mode selector gains "Single Layer" as an option (only shown when ≥1 layer exists, same rule as Layers). Selecting it reveals a dropdown of this upload's labeled layers; selecting a layer swaps in that layer's isolated preview via htmx, matching the existing swap-fragment pattern.
- Submitting in this mode creates a Job with exactly one Pass, for the chosen layer number — reuses the Job/Pass creation plumbing from `axicontrol-print-page-job-creation`, just with a `layer_number` set and no sequencing/awaiting-next-pass involved (unlike Layers mode's job which auto-includes every discovered layer).
- `buildAxicliArgs` already accepts a `layerNumber *int` (`internal/api/jobs_run.go`) for Layers-mode Passes — Single Layer mode's one Pass should invoke `axicli` the same way (`--mode layers --layer N`), it's the Job-shape (one Pass vs. one-per-layer) that differs, not the underlying `axicli` call.

## Acceptance criteria

- [ ] Single Layer appears as a Plot Mode option only when the upload has ≥1 discovered layer
- [ ] Choosing a layer from the dropdown updates the visible preview to show only that layer's geometry
- [ ] The per-layer preview is served via a dedicated endpoint through `<img>`, not inlined SVG
- [ ] Submitting creates a Job with exactly one Pass for the chosen layer, and no other layers are touched
- [ ] The resulting Pass invokes `axicli` with `--mode layers --layer N` for the chosen layer number

## Blocked by

- axicontrol-print-page-job-creation
- axicontrol-layer-labels
