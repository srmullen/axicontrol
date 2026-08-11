---
name: axicontrol-layer-labels
description: DiscoverLayers returns raw layer label text; print page shows a labeled layer list
lane: done
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
assigned-to: claude
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

- [x] `DiscoverLayers` (or a new function) returns each distinct layer number together with the raw label string(s) that produced it
- [x] The print page lists an upload's layers by number and label when ≥1 layer exists
- [x] Nothing is shown when the upload has 0 layers
- [x] Existing Layers-mode Job creation (numbers only) is unaffected by the return-shape change

## Blocked by

- axicontrol-print-page-preview

## Execution Report

**Date:** 2026-08-11

- `internal/svg/layers.go`: `DiscoverLayers` now returns `[]Layer` (`{Number int; Labels []string}`) instead of `[]int`. Label collection is via a new `addLayerLabel` helper, keyed by number, appending each `<g>`'s raw `inkscape:label` in document order and de-duping exact repeats (`slices.Contains`) — the "distinct raw label strings that collapsed into each number" the ticket asked for. Numbers are still returned in ascending order, same as before.
- New exported `svg.StripNumberPrefix(label string) string` strips a label's leading number + immediate hyphen/whitespace separator (e.g. `"5-red"` → `"red"`) — used only by the print page's display formatting, not by `DiscoverLayers` itself, which keeps returning the raw, unprocessed label text per the ticket's wording.
- `internal/api/jobs.go`: `discoverLayersForFile` now returns `[]svg.Layer`. Its one Job-creation caller (`createJobForFile`, layers-mode branch) is otherwise unaffected — a new `layerNumbersOf` helper extracts just the `[]int` that `insertJobAndPasses`/`passLayerNumbersFor` need, so Job creation's behavior (one Pass per layer number) is unchanged.
- `internal/api/print.go`: new `layerView{Number int; Labels string}` and `buildLayerViews` build the print page's display rows — each `svg.Layer`'s raw labels get `StripNumberPrefix`'d and comma-joined (e.g. `"5-red"`/`"5-outlines"` → `"red, outlines"`), since the number is already shown alongside. `printPageView` gained a `Layers []layerView` field alongside the existing `HasLayers` (both now derived from the same single `discoverLayersForFile` call — no extra file read).
- `internal/api/templates/print.html` (`print_content`): a `{{if .Layers}}` block renders `<h2>Layers</h2>` + a plain `<ul>` of `"{number} — {labels}"` (number alone, no em dash, when a layer's labels strip to nothing) between the preview image and the existing submission form. Absent entirely when the Upload has 0 discovered layers, consistent with the mode-selector's hiding rule from `axicontrol-print-page-job-creation`.
- New/updated tests: `internal/svg/layers_test.go` covers the new `Layer` shape (existing tests updated to compare numbers via a `layerNumbers` helper) plus label collection specifically — multiple distinct labels collapsing into one number, exact-duplicate-label de-duping, labels staying separate across numbers, and `StripNumberPrefix` (including the bare-number-strips-to-empty edge case). `internal/api/print_test.go` adds tests for the layer list appearing with correct `"N — label"` text, the multi-label-per-number join format, and its absence for a layerless upload.

**Verification:** `go build`, `go vet`, `golangci-lint run`, and `go test ./...` all clean. Ran the real service locally and uploaded an SVG with two `<g>` layers sharing number 5 (labels `"5-red"`/`"5-outlines"`) plus a `"1 black"` layer; the print page rendered exactly `<li>1 — black</li><li>5 — red, outlines</li>`, matching the ticket's own example format.

**Found and fixed during `/code-review` (Standards + Spec axes) before committing:** Spec review found no issues — all 4 acceptance criteria implemented and tested, no scope creep (no per-layer selection/preview), and confirmed the raw-vs-stripped label handling stays correctly split between `DiscoverLayers` (raw) and display-only `StripNumberPrefix`. It flagged one unverified edge case — a bare-number label (e.g. `"5"`) stripping to empty text — which I added a test for (`TestPrintPageListsBareNumberLayerWithoutDanglingSeparator`); the template already handled it correctly (`{{if .Labels}}` guards the em dash), just untested. Standards review found no hard violations, but four real judgement calls I fixed rather than left: (1) dropped the `HasLayers bool` field entirely — it was redundant with `len(Layers) > 0`, and the template now uses `{{if .Layers}}` for both the mode selector and the layer list; (2) added an exported `svg.Numbers([]Layer) []int` and deleted two near-duplicate one-off projections (`jobs.go`'s `layerNumbersOf`, a test-only `layerNumbers` helper) that did the same "extract .Number into a slice" in three places; (3) unified the two layer-label-parsing regexes into one (`layerNumberPattern` now both captures the number and consumes its separator, so `StripNumberPrefix` reuses it instead of maintaining a second near-identical pattern); (4) replaced a fragile whole-page `require.NotContains(body, "<ul>")` assertion with just the more specific `<h2>Layers</h2>` check, so an unrelated future `<ul>` elsewhere on the page can't fail this test for the wrong reason.
