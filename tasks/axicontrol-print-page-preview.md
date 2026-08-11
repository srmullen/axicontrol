---
name: axicontrol-print-page-preview
description: New per-upload print page showing whole-document preview; replaces inline uploads-list preview
lane: done
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
assigned-to: claude
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

- [x] Clicking an upload in the list navigates to a new page showing that file's whole-document preview
- [x] The preview is rendered via `<img>`, not inlined SVG
- [x] The uploads list no longer has an inline preview panel or "Preview" button — clicking the row is now the way to view a file
- [x] Deleting an upload still works from the uploads list unaffected

## Blocked by

None - can start immediately

## Execution Report

**Date:** 2026-08-11

- New `internal/api/print.go`: `handleUploadPrintPage` loads the upload by id (reusing `uploadIDFromPath`/`loadUploadOrNotFound` from `uploads.go`, no duplication) and renders a new full page via `renderPage`.
- New `internal/api/templates/print.html` (`print_content`): heading + `<img src="/uploads/{id}/content">`, same `<img>`-only approach ADR-0010 already established, reusing the existing sanitized-content endpoint unchanged.
- Route table (`server.go`): `GET /uploads/{id}/print` replaces the old `GET /uploads/{id}` inline-preview fragment route.
- Removed entirely (not just left unused): `handleShowUpload` (`uploads.go`), the `upload_preview` template and `#upload-preview` div (`uploads.html`), and the "Preview" button. The upload row's filename is now a link (`<a href="/uploads/{id}/print">`) instead of an htmx-swap button — clicking it navigates to the new page.
- Delete was untouched — same route, handler, and button as before.
- New/updated tests: `print_test.go` covers the preview rendering (`<img>`, not `<object>`/`<iframe>`), a missing-upload 404, and an invalid-id 400. `uploads_test.go`'s old `TestUploadPreviewRendersImgTag` (which hit the now-removed route) was replaced with `TestUploadRowLinksToPrintPageNotInlinePreview`, asserting both the new link *and* the absence of the old preview affordances.

**Verification:** `go build`, `go vet`, `golangci-lint run`, `go test ./...` all clean. Also ran the real service locally (`go run ./cmd/axicontrold`) and confirmed via curl: uploading a file, seeing the print-page link in the list, following it to a 200 with the expected `<img>` tag, and a 404 for a nonexistent upload id.

**Found and fixed during `/code-review` (Standards + Spec axes) before committing:**
- Standards review flagged that `handlePrintPage` broke this package's handler-naming convention (every other handler names its resource, e.g. `handleUploadContent`, `handleDeleteUpload`) — renamed to `handleUploadPrintPage`.
- Spec review found no issues: route matches the ticket's suggested `GET /uploads/{id}/print` exactly, old route/handler/template confirmed fully removed rather than left dead, delete confirmed untouched, and no scope creep into later tickets (job creation, mode selector, layer content correctly absent from this change).
