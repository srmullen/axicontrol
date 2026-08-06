---
name: axicontrol-layers-mode
description: Multi-layer plot jobs with per-layer Passes
lane: todo
tags: ready-for-agent, user-management
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-print-whole-job
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (Job/Pass model, `layers` mode).

## What to build

Extend Jobs to `layers` mode: one Pass per auto-discovered layer number, printed one at a time with an explicit user trigger between them (no auto-advance).

- Parse the uploaded SVG's layer names for AxiDraw's numeric-prefix convention (e.g. `1 black`, `2 red`) to auto-discover Passes at Job-submission time.
- Job status gains `awaiting-next-pass`: reached when a Pass completes and a next Pass exists.
- An explicit "advance to next Pass" action starts the next Pass's `axicli` invocation; nothing starts automatically. This is an htmx button on the Job status page ([ADR-0012](../docs/adr/0012-frontend-stack.md)).
- Job reaches `complete` only once its last Pass completes.
- Layers mode reuses whole-mode's status machinery, pause/resume, and config resolution unchanged per Pass.

## Acceptance criteria

- [ ] Uploading a multi-layer SVG and submitting a `layers` Job creates one Pass per discovered layer, in layer order
- [ ] After a Pass completes with a next Pass pending, the Job sits in `awaiting-next-pass` indefinitely until the user triggers the advance — it does not auto-start
- [ ] Triggering advance starts the next layer's `axicli` invocation
- [ ] A `layers` Job reaches `complete` only after its final Pass completes
- [ ] An individual Pass within a `layers` Job can be paused/resumed (reuses axicontrol-pause-resume)

## Blocked by

- axicontrol-print-whole-job
