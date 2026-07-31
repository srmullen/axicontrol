---
name: axicontrol-architecture-spec-03-configuration-model
description: How AxiDraw CLI configuration options are modeled and stored
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

How is AxiDraw CLI configuration (pen up/down height, speed, etc.) modeled and stored? Decide: named reusable presets vs. per-job overrides vs. both, where they're persisted, and how a job selects/combines them.

## Execution Report

**Date:** 2026-07-31

### Decision

The CLI's own option set ([docs](https://axidraw.com/doc/cli_api/)) splits naturally into hardware-fixed values and plot-affecting values; axicontrol's model follows that split and adds the named-preset system the CLI itself doesn't have:

**Device Config** (singleton — there is one physical AxiDraw)
- `model`, `penlift` — hardware/servo configuration for this specific unit.

**Preset** (named, reusable)
- The plot-affecting scalar/boolean options: `speed_pendown`, `speed_penup`, `accel`, `pen_pos_down`, `pen_pos_up`, `pen_rate_lower`, `pen_rate_raise`, `pen_delay_down`, `pen_delay_up`, `const_speed`, etc.
- `name` + description, e.g. "draft", "fine-detail".

**Pass** (extends the [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md)) references exactly one Preset, plus an optional set of key/value overrides applied on top for that Pass only — mirrors the CLI's own args-override-config-file precedence, without requiring a new named Preset for a one-off tweak.

**SVG sanitization**: the CLI's precedence is SVG layer settings > CLI args > config file > defaults — meaning a hand-authored SVG could silently override whatever Preset the UI shows. axicontrol strips any AxiDraw-specific per-layer option overrides from uploaded SVGs (see [file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md)) so the displayed Preset + overrides is always the full, sole source of truth.

**At invocation**: axicontrol resolves Device Config + the Pass's Preset + its overrides into a single generated config file (or equivalent CLI args) passed to `axicli` for that Pass.

### Consequence

Device Config and Presets need a datastore, same as Jobs/Passes — but which storage technology backs axicontrol's structured records at all hasn't been decided yet, and it's foundational/cross-cutting rather than specific to configuration. Surfaces as a new ticket: [primary datastore](./axicontrol-architecture-spec-09-primary-datastore.md).
