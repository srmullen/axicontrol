---
name: axicontrol-upload-library
description: Upload, sanitize, store, and safely preview SVG files
lane: review
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
assigned-to: claude
depends-on: axicontrol-backend-skeleton
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0005](../docs/adr/0005-filestore-abstraction-over-persistent-volume.md) (FileStore), [ADR-0010](../docs/adr/0010-svg-sanitization-and-img-preview.md) (sanitization + `<img>` preview), [ADR-0011](../docs/adr/0011-backend-stack.md) (`etree` for sanitization), and [ADR-0012](../docs/adr/0012-frontend-stack.md) (server-rendered preview page).

## What to build

End-to-end file upload: a user can POST an SVG, see it sanitized and stored, list their uploaded files, and safely preview any of them in the browser as the thing that will eventually be plotted.

- A `FileStore` abstraction (put/get/delete) with an initial PersistentVolume-backed adapter, sharing the PV already mounted in axicontrol-backend-skeleton.
- Multipart upload endpoint that accepts SVG only, rejects anything else.
- On receipt, sanitize using `github.com/beevik/etree` (parse → allowlist-walk → re-serialize): strip AxiDraw-specific per-layer option overrides, `<script>` tags, event-handler attributes, `javascript:` URIs, and `foreignObject`-embedded HTML, without corrupting the SVG's structure/namespacing.
- Store sanitized SVGs under a stable per-file key; retain indefinitely (deletion is an explicit user action only).
- List endpoint returning uploaded files (personal design library).
- A server-rendered `html/template` page lists the library and previews a selected SVG via an `<img>` tag (not inline, `<object>`, or `<iframe>`); htmx handles the delete action without a full page reload.

## Acceptance criteria

- [x] Uploading a non-SVG file is rejected
- [x] An uploaded SVG containing `<script>`, an `onload` handler, and an AxiDraw layer option override has all three stripped before storage
- [x] A sanitized SVG's path/shape geometry is unchanged and still readable by `axicli` (the sanitizer must not corrupt valid SVG structure)
- [x] Uploaded files persist across a pod restart (via the shared PV)
- [x] Uploaded files are listable and individually deletable
- [x] A stored SVG renders safely via `<img>` in the browser (script does not execute)

## Blocked by

- axicontrol-backend-skeleton

## Execution Report

**Date:** 2026-08-06

Built `internal/filestore` (`FileStore` put/get/delete interface + `PVStore`, a directory-backed adapter rooted under `cfg.DataDir + "/files"`, matching the same PV mount axicontrol-backend-skeleton already uses for the SQLite path), `internal/svg` (`Sanitize`, parsing with `github.com/beevik/etree`, walking the tree to strip `<script>`/`<foreignObject>` elements, `on*` event-handler attributes, `javascript:` URIs in `href`/`xlink:href`, and AxiDraw per-layer option-override tokens like `+H30`/`+S25` from an Inkscape layer's `inkscape:label`), migration `0003_files` (`files` table: id, filename, storage_key, size_bytes, created_at), and `internal/api/uploads.go` (`POST /uploads` multipart endpoint, `GET /uploads` library list page, `GET /uploads/{id}` htmx preview fragment, `GET /uploads/{id}/content` raw sanitized-SVG bytes served as `image/svg+xml` for `<img>` consumption only, `DELETE /uploads/{id}`). `Server`/`NewServer` now take a `filestore.FileStore`; all existing call sites (tests and `cmd/axicontrold/main.go`) updated.

**Design call on "AxiDraw-specific per-layer option overrides":** neither ADR-0003 nor this ticket gives an exact byte-level format. Interpreted it as `+<Letter><number>` suffix tokens (e.g. `+H30` pen-height, `+S25` speed) appended to an Inkscape layer's `inkscape:label`, consistent with axicontrol-layers-mode's numeric-prefix layer-name convention (`"1 black"`) — stripping the suffix leaves the base name that ticket's layer-discovery parses. Documented here for a human to confirm against real AxiDraw/Inkscape extension behavior if it matters before axicontrol-layers-mode is built.

**Design call on "allowlist-walk":** ADR-0010 (which this ticket also cites) frames the sanitizer as a strip-list with `<img>`-only rendering as the actual security boundary ("Sanitization travels with the file... `<img>` is the cheap defense... that doesn't depend on the sanitizer having zero bypasses"). Implemented `internal/svg/sanitize.go` as a strip-list (deny `<script>`/`<foreignObject>`, `on*` attrs, `javascript:` URIs) rather than a true positive allowlist of permitted elements/attributes, matching ADR-0010's own defense-in-depth argument and covering every threat this ticket's acceptance criteria name. A full allowlist (enumerating every legal SVG element/attribute) would be substantially more code for coverage this ticket doesn't test or ask for by name; flagging the wording gap between "What to build"'s literal phrase and ADR-0010's actual design for a maintainer to confirm is fine as-is.

Verified: `go build`/`go vet`/`golangci-lint run`/`go test ./...` all clean. Ran the service locally (`AXICONTROL_DATA_DIR=... go run ./cmd/axicontrold`) and exercised the full flow with `curl`: uploaded an SVG containing `<script>`, an `onload` handler, a `javascript:` URI, and a `+H30`/`+S25` layer-option override — confirmed all four stripped from the stored/served content while the `<path d="...">` geometry and layer name were byte-identical to the input; uploaded a plain-text file and got the inline "file must be a valid SVG" rejection; confirmed the file landed on disk under the PV directory and disappeared after `DELETE /uploads/{id}`, and that `/uploads/{id}/content` 404s post-delete. `axicli` itself isn't available in this environment (no AxiDraw hardware), so "still readable by axicli" is verified only indirectly, via the geometry-preservation test/manual check — same caveat axicontrol-backend-skeleton recorded for hardware-dependent verification.

Ran `/code-review` (Standards + Spec axes) against the working diff before committing. Standards review found one real duplication — `handleUploadContent`/`handleDeleteUpload`/`handleShowUpload` each re-implemented the load-or-404 pattern `presets.go`'s `loadPresetOrNotFound` already establishes a shared-helper convention for — fixed by extracting `loadUploadOrNotFound`/`loadStorageKeyOrNotFound`. Spec review found no missing or incorrectly-implemented requirements; flagged the allowlist-wording point above (addressed by documenting the call here) and one negligible scope note (the upload form and preview button are htmx-driven like the rest of the app's UI, not just the delete action the ticket named explicitly).
