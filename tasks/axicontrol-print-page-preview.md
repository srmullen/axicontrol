---
name: axicontrol-print-page-preview
description: New per-upload print page showing whole-document preview; replaces inline uploads-list preview
lane: todo
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md) and the `CONTEXT.md` glossary.

## What to build

Add a new page scoped to a single Upload, reachable by clicking that upload in the uploads list, and remove the list's current inline preview.

- New route (e.g. `GET /uploads/{id}/print`) rendering a page with a whole-document preview via `<img>`, reusing the existing sanitized-SVG-serving endpoint (`GET /uploads/{id}/content`) per [ADR-0010](../docs/adr/0010-svg-sanitization-and-img-preview.md) — do not inline-embed the SVG.
- Clicking an upload row in the uploads list navigates to this page instead of swapping an inline preview fragment into the current page.
- Remove the existing inline preview panel/button and its htmx fragment route/usage from `templates/uploads.html`.
- This ticket is preview-only — no job creation, no mode selector, no layer content. Those come in later tickets.

## Acceptance criteria

- [ ] Clicking an upload in the list navigates to a new page showing that file's whole-document preview
- [ ] The preview is rendered via `<img>`, not inlined SVG
- [ ] The uploads list no longer has an inline preview panel or "Preview" button — clicking the row is now the way to view a file
- [ ] Deleting an upload still works from the uploads list unaffected

## Blocked by

None - can start immediately
