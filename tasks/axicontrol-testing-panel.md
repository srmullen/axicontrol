---
name: axicontrol-testing-panel
description: Hardware self-test/jog panel and per-file plot dry-run
lane: todo
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-backend-skeleton,axicontrol-print-whole-job
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0004](../docs/adr/0004-carriage-position-tracking.md) (carriage position tracking) and [ADR-0012](../docs/adr/0012-frontend-stack.md) (htmx jog panel UI).

## What to build

Two related but independent testing capabilities:

**Hardware self-test / jog panel** (no uploaded file involved):
- Connectivity/status via `sysinfo`.
- Pen test via `cycle`/`toggle`.
- Manual alignment via `align`.
- Home via `walk_home`.
- Move-to-coordinate: axicontrol tracks carriage position in memory, zeroed on `walk_home` or motor-enable, translating a "go to (x, y)" request into relative `walk_x`/`walk_y` deltas. Explicitly not persisted — a pod restart loses it until the next home.
- All jog panel controls are htmx forms/buttons on an `html/template` page, posting to the corresponding endpoints without a full page reload.

**Plot dry-run** (per uploaded file, reuses Job/Pass/Preset from axicontrol-print-whole-job):
- Adds `preview` (+ `report_time`) to a Pass's resolved config, simulating geometry/timing without lowering the pen or moving the device for real.
- Surfaces a time estimate to the user before they commit to a real print.

## Acceptance criteria

- [ ] `sysinfo`, `cycle`/`toggle`, `align`, and `walk_home` are each triggerable via the API and reflect real device behavior
- [ ] A move-to-coordinate request after homing moves the carriage to the correct physical position
- [ ] A pod restart resets tracked position (next move-to-coordinate is wrong until re-homed) — this is expected, not a bug
- [ ] Running a dry-run on an uploaded Job/Preset combination returns a time estimate without the pen touching paper
- [ ] Dry-run reuses the same config resolution (Device Config + Preset + overrides) as a real print

## Blocked by

- axicontrol-backend-skeleton
- axicontrol-print-whole-job
