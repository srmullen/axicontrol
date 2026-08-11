---
name: axicontrol-uploads-home-dragdrop
description: Uploads page becomes the app home page, with drag-and-drop upload support
lane: done
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
assigned-to: claude
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md) and the `CONTEXT.md` glossary (both new this session).

## What to build

Make the uploads page the application's home page, and let uploads be added by dragging a file onto the page in addition to the existing file-picker.

- `GET /{$}` serves the uploads list instead of redirecting to `/device-config`. Update the shared nav (`templates/layout.html`) so Uploads is the first/home item.
- Add a drag-and-drop drop zone to the uploads page's existing upload form. Dropping a single `.svg` file uploads it via the same `POST /uploads` flow (multipart, sanitized, 10MiB cap) already in place — no new backend validation needed, this is a UI affordance on top of the existing endpoint.
- Keep the classic `<input type="file">` picker working as a fallback (accessibility, browsers/automation without native drag-and-drop support).
- Multi-file drop is explicitly out of scope for this ticket — a drop of more than one file should be handled the same way the current form already handles being given the wrong thing (reject/ignore gracefully), not silently upload only the first file.

## Acceptance criteria

- [x] Visiting `/` shows the uploads list (no redirect to device-config)
- [x] Nav reflects Uploads as the home/first item
- [x] Dragging a single `.svg` file onto the uploads page uploads it, and it appears in the list without a page reload (or with one, consistent with existing htmx patterns)
- [x] The existing file-picker input still works unchanged
- [x] Dropping multiple files at once does not silently upload only one of them unnoticed — it's rejected or clearly surfaced, not swallowed

## Blocked by

None - can start immediately

## Execution Report

**Date:** 2026-08-11

- `GET /{$}` (`internal/api/server.go`, `handleIndex`) now redirects to `/uploads` instead of `/device-config`; nav in `templates/layout.html` reordered so Uploads is first (rest of the order otherwise unchanged: Device Config, Presets, Jobs, Webhooks, Testing).
- `templates/uploads.html`'s upload form is now wrapped in `<div id="upload-dropzone">` alongside a `<p id="upload-dropzone-error">` target and a hint line; the file-picker `<input>`/`<form>` themselves are untouched, still posting to the existing `POST /uploads` endpoint unchanged.
- New `internal/api/static/app.js` (first app-specific JS file; `sse.js` was vendored) adds document-delegated `dragover`/`drop` listeners scoped to `#upload-dropzone` via `closest()` — delegated on `document` rather than bound directly to the dropzone because htmx's `hx-swap="outerHTML"` on `#uploads-section` replaces that element (and any listener on it) after every upload. On drop: a single file is assigned to the existing file input via `DataTransfer`, then `form.requestSubmit()` fires a real `submit` event for htmx to intercept (no new backend validation — this rides on the existing multipart/sanitize/10MiB-cap path). Dropping more than one file sets a visible error message and returns before touching the input, so nothing is silently uploaded.
- No new backend endpoint or validation was needed — this is a client-side affordance over the existing `POST /uploads` handler.

**Verification:** `go build`, `go vet`, `golangci-lint run`, and `go test ./...` all clean. Ran the real service locally (`go run ./cmd/axicontrold`) and confirmed via curl that `/` redirects to `/uploads` and the rendered page contains the dropzone markup. The Chrome browser-automation tool wasn't connected in this environment, so the actual drag-and-drop interaction couldn't be exercised in a real browser; instead, `app.js`'s drop-handler logic was traced by hand and exercised against a minimal hand-rolled DOM fake (single file → `requestSubmit()` called, input populated; multiple files → no submit, error text set; drop outside the dropzone → ignored entirely) to confirm the branching is correct. This is a lower-confidence substitute for real browser verification and worth a manual smoke-test before this ships to a real device.

**Found and fixed during `/code-review` (Standards + Spec axes) before committing:**
- Spec review flagged that the nav reorder moved `Jobs` ahead of `Device Config`/`Presets` too, which wasn't asked for (the ticket only requested Uploads-first) — reverted to only moving Uploads to the front, everything else keeps its original relative order.
- Standards review flagged a new test (`TestUploadsSectionHasDropzoneAroundExistingForm`) whose name implied it verified the form was nested inside the dropzone div, but its assertions only checked substring presence anywhere in the page. Renamed to `TestUploadsSectionHasDropzoneWrappingForm` and strengthened it to assert ordering (dropzone open tag → form → file input → closing `</div>`) so the name matches what's actually verified.
- Standards review also noted new CSS introduced `#ccc`/`#666` ad hoc instead of reusing the existing `#ddd` border color already used elsewhere; aligned the dropzone border to `#ddd`.
- Not changed: the unconditional (CSS-hidden-when-empty) error `<p>` deviates from this codebase's usual `{{if .Error}}` template-conditional pattern — flagged as a judgement call, not a violation, since the client-side JS needs a stable DOM node to write into before any error exists; left as-is.
