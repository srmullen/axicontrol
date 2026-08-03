---
name: axicontrol-architecture-spec-10-svg-preview-safety
description: How uploaded SVGs are served for direct browser rendering safely, given they can embed script/event-handler content
lane: todo
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
parent: axicontrol-architecture-spec
---

## Question

Uploaded SVGs are served directly by the single backend ([service shape](./axicontrol-architecture-spec-07-service-shape.md)) so the browser can render "the file that will be plotted" (a README requirement) — but SVG can embed `<script>` tags and event-handler attributes, making direct rendering of user-uploaded content an XSS surface. How is this addressed: sanitizing scripts/event-handlers at upload time (alongside the AxiDraw-specific option stripping already decided in [configuration model](./axicontrol-architecture-spec-03-configuration-model.md)), serving with a restrictive Content-Security-Policy / sandboxed rendering context, or both?
