# Context

## Glossary

- **Upload** — an SVG file the user has submitted, sanitized, and stored (`internal/api/uploads.go`, `internal/svg/sanitize.go`). Identified by UUID.
- **Layer** — a group of SVG geometry sharing the same leading integer in its Inkscape `inkscape:label` (e.g. `"5-red"` and `"5-outlines"` both belong to Layer 5). Discovered per-Upload via `svg.DiscoverLayers`. A Layer is identified by its number; multiple `<g>` elements can collapse into one Layer number. The UI must show the original label text(s) that collapsed into a Layer number, not just the bare number — a Layer with only a number and no label is not considered identifiable to the user.
- **Job** — one plotting request against an Upload, using a chosen Plot Mode and Preset. Composed of one or more Passes (`internal/store`, ADR-0002).
- **Pass** — one physical plot run within a Job — one continuous plot from start to finish with no assumed pen change mid-Pass.
- **Plot Mode** — how a Job's Passes are derived from the Upload's Layers. Three modes:
  - **Whole** — one Pass, plots the entire document ignoring layer boundaries.
  - **Layers** — one Pass per discovered Layer, run in sequence; the operator manually advances between Passes (assumed pen swap between each). Plots *every* Layer in the Upload.
  - **Single Layer** *(new)* — one Pass, plots exactly one chosen Layer and nothing else. Distinct from Layers mode: no sequencing, no other Layers touched.
- **Device-busy** — the AxiDraw can run at most one Job at a time; while a Job is active (including paused between Layers-mode Passes), no new Job can be created against any Upload, and this state must be visible from any Upload's print page, not just the one whose Job is running.
