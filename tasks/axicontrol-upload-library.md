---
name: axicontrol-upload-library
description: Upload, sanitize, store, and safely preview SVG files
lane: backlog
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
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

- [ ] Uploading a non-SVG file is rejected
- [ ] An uploaded SVG containing `<script>`, an `onload` handler, and an AxiDraw layer option override has all three stripped before storage
- [ ] A sanitized SVG's path/shape geometry is unchanged and still readable by `axicli` (the sanitizer must not corrupt valid SVG structure)
- [ ] Uploaded files persist across a pod restart (via the shared PV)
- [ ] Uploaded files are listable and individually deletable
- [ ] A stored SVG renders safely via `<img>` in the browser (script does not execute)

## Blocked by

- axicontrol-backend-skeleton
