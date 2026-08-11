---
name: axicontrol-uploads-home-dragdrop
description: Uploads page becomes the app home page, with drag-and-drop upload support
lane: todo
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
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

- [ ] Visiting `/` shows the uploads list (no redirect to device-config)
- [ ] Nav reflects Uploads as the home/first item
- [ ] Dragging a single `.svg` file onto the uploads page uploads it, and it appears in the list without a page reload (or with one, consistent with existing htmx patterns)
- [ ] The existing file-picker input still works unchanged
- [ ] Dropping multiple files at once does not silently upload only one of them unnoticed — it's rejected or clearly surfaced, not swallowed

## Blocked by

None - can start immediately
