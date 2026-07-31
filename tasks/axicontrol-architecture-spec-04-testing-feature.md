---
name: axicontrol-architecture-spec-04-testing-feature
description: What 'testing the axidraw' means as an app feature and how it's exposed
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

What does "testing the axidraw" mean as an app feature? AxiDraw CLI exposes dry-run/alignment-style modes (e.g. `--mode align`, pen-up-only passes) — decide which CLI test/calibration capabilities get surfaced and how they're exposed through the API.

## Execution Report

**Date:** 2026-07-31

### Decision

Two distinct features, both grounded in the [AxiDraw CLI docs](https://axidraw.com/doc/cli_api/):

**Hardware self-test / jog panel** — standalone actions, no uploaded file involved, all routed through the same node-pinned pod as everything else ([device access](./axicontrol-architecture-spec-01-device-access.md)):
- Connectivity/status check via `sysinfo`.
- Pen test via `cycle`/`toggle`.
- Manual alignment via `align` (disengages motors, raises pen — e.g. for loading paper or seating a pen).
- Home via `walk_home`.
- **Move-to-coordinate**: the CLI has no absolute-position concept — only relative `walk_x`/`walk_y` and `walk_home` (return to the reference point set when motors were last enabled), with no position readback. axicontrol tracks the carriage's position itself, zeroed whenever `walk_home` runs or the device is freshly connected/motors enabled (the only reliable reference point the hardware gives), and translates a "go to (x, y)" request into the relative `walk_x`/`walk_y` delta needed to get there. This tracked position is best-effort/in-memory in the node-pinned pod — a pod restart loses it until the next home.

**Plot dry-run** — per uploaded file, reuses the Job/Pass/Preset machinery from [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md) and [configuration model](./axicontrol-architecture-spec-03-configuration-model.md): adds `preview` (+ `report_time`) to the resolved config for a Pass invocation, simulating geometry/timing without lowering the pen or moving for real, surfacing a time estimate before the user commits to a real print.

### Consequence

No new tickets — both features compose entirely out of already-decided pieces (node-pinned pod, Job/Pass/Preset). The move-to-coordinate carriage-position tracking is explicitly ephemeral/in-memory rather than a new persistence requirement.
