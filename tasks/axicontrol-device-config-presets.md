---
name: axicontrol-device-config-presets
description: Manage Device Config and named Presets for plot configuration
lane: backlog
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-backend-skeleton
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0003](../docs/adr/0003-device-config-and-presets.md) (Device Config & Presets).

## What to build

CRUD for the two configuration concepts that later feed plot jobs: a singleton Device Config, and named, reusable Presets.

- Device Config: singleton record for hardware-fixed values (`model`, `penlift`). Get/update endpoints — there's exactly one, no create/delete.
- Preset: named, reusable records holding the plot-affecting values (`speed_pendown`, `speed_penup`, `accel`, `pen_pos_down`, `pen_pos_up`, `pen_rate_lower`, `pen_rate_raise`, `pen_delay_down`, `pen_delay_up`, `const_speed`, etc.), plus a `name` and description. Full CRUD.
- Persisted in the embedded SQLite from axicontrol-backend-skeleton.
- No plotting happens yet — this ticket is configuration data management only, consumed later by axicontrol-print-whole-job.

## Acceptance criteria

- [ ] Device Config can be read and updated; there is exactly one record, never created/deleted via the API
- [ ] Presets can be created, listed, updated, and deleted, each with a unique name
- [ ] Config and Presets persist across a pod restart

## Blocked by

- axicontrol-backend-skeleton
