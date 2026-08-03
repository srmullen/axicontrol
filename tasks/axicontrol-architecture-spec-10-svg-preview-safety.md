---
name: axicontrol-architecture-spec-10-svg-preview-safety
description: How uploaded SVGs are served for direct browser rendering safely, given they can embed script/event-handler content
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

Uploaded SVGs are served directly by the single backend ([service shape](./axicontrol-architecture-spec-07-service-shape.md)) so the browser can render "the file that will be plotted" (a README requirement) — but SVG can embed `<script>` tags and event-handler attributes, making direct rendering of user-uploaded content an XSS surface. How is this addressed: sanitizing scripts/event-handlers at upload time (alongside the AxiDraw-specific option stripping already decided in [configuration model](./axicontrol-architecture-spec-03-configuration-model.md)), serving with a restrictive Content-Security-Policy / sandboxed rendering context, or both?

## Execution Report

**Date:** 2026-08-03

### Decision

**Both** sanitization and a restricted rendering context — not either alone.

**Sanitize at upload time**: extend the sanitization pass [configuration model](./axicontrol-architecture-spec-03-configuration-model.md) already established (which strips AxiDraw-specific per-layer option overrides) to also strip `<script>` tags, event-handler attributes (`onload`, `onclick`, etc.), `javascript:` URIs, and `foreignObject`-embedded HTML. Cost is negligible: `axicli` only reads path/shape data, so scripts have zero effect on plot output — there's no correctness trade-off to weigh against the safety win.

**Serve the preview via `<img>`**: the browser preview renders the SVG through an `<img>` tag, not inline embedding, `<object>`, or `<iframe>`. Confirmed against current browser behavior: SVGs loaded via `<img>` run in browsers' strictest built-in security context — script tags and event-handler attributes never execute there, regardless of content — so this satisfies the "restrictive rendering context" half of the question without building or maintaining a custom CSP/sandboxing scheme.

**Why both, not one**: `<img>`'s script-blocking is context-specific — it only applies when the file is rendered through that tag. The same file opened directly (a raw download link, "open in new tab") still executes embedded scripts under axicontrol's own origin, and these SVGs are retained indefinitely as a personal design library ([file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md)), making future direct-access paths (a download endpoint, re-selection as a job source) plausible. Sanitization is the defense that travels with the file to any such future consumer; `<img>` is the cheap, built-in defense for the preview path specifically that doesn't depend on the sanitizer having zero bypasses. Neither alone covers both cases.

**Checkpoint files inherit this for free**: [checkpoint persistence](./axicontrol-architecture-spec-08-checkpoint-persistence.md)'s checkpoint SVGs are `axicli`'s output from reading the already-sanitized upload as input, adding only resume-position metadata — `axicli` never reintroduces script content, so no separate sanitization step is needed for checkpoints.

### Consequence

None — this was the last open ticket on the map. No new tickets surfaced.
