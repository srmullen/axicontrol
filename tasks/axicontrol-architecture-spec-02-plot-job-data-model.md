---
name: axicontrol-architecture-spec-02-plot-job-data-model
description: 'Data model for a plot job: states, and how AxiDraw layers relate to a job'
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

What is the data model for a plot job? Define the states a job moves through (e.g. queued, printing, paused, resumed, complete, failed, cancelled), how AxiDraw CLI "layers" relate to a job (one job with multiple layers printed in sequence?), and what persists across a pause/resume cycle.

## Execution Report

**Date:** 2026-07-31

### Decision

The AxiDraw CLI's own model is the basis: a `whole` invocation plots every layer at once, a `layers` invocation plots exactly one numbered layer, and either can be paused mid-run (SIGINT) with progress checkpointed into an output SVG for `res_plot`/`res_home` resume ([AxiDraw CLI docs](https://axidraw.com/doc/cli_api/)). The data model unifies both cases:

**Job**
- References the uploaded file.
- `mode`: `whole` or `layers`.
- An ordered list of **Passes** — a `whole` job always has exactly 1 Pass; a `layers` job has one Pass per layer number, auto-discovered by parsing the SVG's layer names for AxiDraw's own numeric-prefix convention (e.g. `1 black`, `2 red`).
- `status`, derived from its Passes: `queued` → `printing` → (`paused` ⇄ `printing`) → `awaiting-next-pass` (current Pass done, a next Pass exists but requires an explicit user trigger — no auto-advance, since layers are typically used for manual pen changes) → `complete`, or `failed` / `cancelled`.

**Pass** (one per CLI invocation)
- Sequence index; layer number (null for `whole` jobs).
- `status`: `pending` → `running` → (`paused` ⇄ `running`) → `complete` / `failed` / `cancelled`.
- A checkpoint file path — the `-o` output SVG `axicli` writes progress into, used for resume.

**Transitions**, tying back to [device access](./axicontrol-architecture-spec-01-device-access.md)'s node-pinned pod:
- Start a Pass → pod spawns `axicli` for that layer/whole-file.
- Pause → pod sends SIGINT to the running subprocess; it finishes the current line segment, writes the checkpoint, exits.
- Resume → pod re-invokes `axicli --mode res_plot` (or `res_home`) against that same checkpoint file.
- Pass completes → next Pass exists: Job → `awaiting-next-pass`, waiting on the user's explicit trigger. No more Passes: Job → `complete`.
- Pass fails → Job → `failed`; retry re-runs that Pass fresh (a crash doesn't necessarily leave a clean resumable checkpoint, unlike a pause).
- Cancel (from `pending`/`paused`/`running`) → Pass and Job → `cancelled`, terminal.

### Consequence

Surfaces a new question not yet ticketed: where Pass checkpoint files persist and whether they need to survive the node-pinned pod restarting or being rescheduled — see [checkpoint file persistence](./axicontrol-architecture-spec-08-checkpoint-persistence.md).
