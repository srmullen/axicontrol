---
name: axicontrol-single-layer-mode
description: 'Single Layer plot mode: pick one labeled layer, preview it live, submit a one-Pass job for it alone'
lane: done
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
assigned-to: claude
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

- [x] Single Layer appears as a Plot Mode option only when the upload has ≥1 discovered layer
- [x] Choosing a layer from the dropdown updates the visible preview to show only that layer's geometry
- [x] The per-layer preview is served via a dedicated endpoint through `<img>`, not inlined SVG
- [x] Submitting creates a Job with exactly one Pass for the chosen layer, and no other layers are touched
- [x] The resulting Pass invokes `axicli` with `--mode layers --layer N` for the chosen layer number

## Blocked by

- axicontrol-print-page-job-creation
- axicontrol-layer-labels

## Execution Report

**Date:** 2026-08-11

- `internal/svg/layers.go`: new `IsolateLayer(data []byte, number int) ([]byte, error)` re-serializes an SVG with every Inkscape layer group removed except the one(s) matching `number` (multiple `<g>`s sharing a number, e.g. `"5-red"`/`"5-outlines"`, are kept together, mirroring `DiscoverLayers`'s own collapsing rule). A layer group nested inside a kept layer is still just "another layer group" and gets stripped if its own number differs — sublayers don't inherit their parent's fate. `isLayerGroup` (extracted from `DiscoverLayers`'s `collectLayers`) and a new `parseSVGDocument` (the parse-and-validate-root preamble both functions share) are reused by both.
- `internal/api/uploads.go`: new `GET /uploads/{id}/layers/{number}/content` (`handleUploadLayerContent`) serves `IsolateLayer`'s output as `image/svg+xml`, same `<img>`-only posture as the existing whole-document endpoint (ADR-0010) — 404s if `number` isn't among the Upload's own `DiscoverLayers` results, rather than silently returning an emptied-out document.
- `internal/api/jobs.go`: `createJobForFile` gained a `mode == "single_layer"` branch alongside `whole`/`layers`, taking a new `singleLayer *int` parameter — validates the chosen layer is one of the Upload's discovered layers (`"layer not found"` if not, `"select a layer"` if omitted), then creates exactly one Pass at that layer number. `passLayerNumbersFor` (one `*int` per Pass to insert) dropped its `mode` parameter entirely — it already generalized to "one Pass per entry in layerNumbers, or one unnumbered Pass if empty," which covers `single_layer`'s one-entry case for free. No changes to `buildAxicliArgs`/`runPass`/`executePass` — a Single Layer Pass carries a `layerNumber` exactly like a Layers-mode Pass does, so `--mode layers --layer N` invocation is already correct as-is.
- `internal/api/print.go` + `templates/print.html`: extracted the print page's preview `<img>` and Plot Mode `<select>` into a `print_preview` fragment (new `GET /uploads/{id}/preview`, `handleUploadPreview`) — selecting Single Layer reveals a Layer `<select>` (labeled per `axicontrol-layer-labels`'s format); selecting a Layer re-renders the fragment with that layer's isolated-preview `<img src>`. Both `<select>`s live outside the Job-submission `<form>` now (so the same live-preview area serves Whole/Layers mode-switches too, not just Single Layer's), wired back in via the form's `hx-include="#print-preview"`. `handleUploadPreview` re-validates any `layer_number` query param against the Upload's own discovered layers, silently falling back to the whole-file image (rather than building a `<img src>` for a nonexistent layer) if it doesn't match — the same defense-in-depth the Job-creation and content-serving paths already have.

**Verification:** `go build`, `go vet`, `golangci-lint run`, and `go test -race ./...` all clean. Ran the real service locally (`go run ./cmd/axicontrold`) against an upload with two numbered layers: confirmed the mode selector gains "Single Layer" only when layers exist, confirmed choosing a layer swaps the preview `<img>` to the isolated single-layer content (verified via `DiscoverLayers` on the response that only the chosen layer remains), confirmed a plain (layerless) upload shows neither the mode selector nor a layer picker, confirmed submitting Single Layer mode creates a Job with exactly one Pass ("1/1") and rejects a submission with no layer chosen ("select a layer") inline. `axicli` isn't installed in this dev environment (same caveat prior tickets recorded), so the created Pass failed at the actual subprocess call — its unit-test coverage (`TestJobSubmitSingleLayerModeCreatesOnePassForChosenLayerOnly`, with a faked `runAxicli`) confirms the invocation itself carries `--mode layers --layer N`.

**Found and fixed during `/code-review` (Standards + Spec axes) before committing:** Standards review found one hard violation — a new `layerNumberFromForm` helper returned a raw, unwrapped `strconv.Atoi` error while its two callers each hardcoded a different, inconsistent user-facing string; fixed by having the helper itself return the ready-to-display message, matching `parseOverridesForm`'s existing convention for this handler family. It also flagged three Fowler-smell judgement calls (Duplicated Code in the SVG parse-and-validate preamble, Primitive Obsession/Repeated Switches on the three-way Plot Mode string, a Data Clump on `createJobForFile`'s new `mode`+`singleLayer` parameter pair) — the first was cheap and low-risk so I fixed it (extracted `parseSVGDocument`); the other two were left as-is, consistent with this repo's existing convention of plain strings for enum-like fields (`status`, `mode`) elsewhere and this ticket's own precedent of leaving similar low-severity smells alone unless the shape actually grows. Spec review found no missing or wrongly-implemented acceptance criteria and confirmed the nested/shared-layer-group handling was correctly tested, but flagged one real gap: the live-preview fragment (`handleUploadPreview`) built its isolated-layer `<img src>` from the raw `layer_number` query param with no check that it belonged to this Upload, unlike the Job-creation and content-serving paths, which could silently show a broken image for a stale/tampered value — fixed by validating it the same way, falling back to the whole-file image when it doesn't match.
