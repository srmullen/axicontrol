---
name: axicontrol-print-whole-job
description: Print a whole-file plot job end to end
lane: todo
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-upload-library,axicontrol-device-config-presets
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (Job/Pass model), [ADR-0003](../docs/adr/0003-device-config-and-presets.md) (config resolution), and [ADR-0012](../docs/adr/0012-frontend-stack.md) (htmx job submission + status page).

## What to build

Print a `whole`-mode Job end to end: pick an uploaded SVG and a Preset (with optional overrides), submit a Job, and watch it actually plot on the real AxiDraw through to completion or failure. This is the core "print" path — layers mode, pause/resume, and notifications are separate follow-on tickets.

- Job: references an uploaded file, `mode: whole`, exactly one Pass.
- Pass: sequence index 0, `status: pending → running → complete/failed`.
- Submitting a Job resolves Device Config + selected Preset + any Pass-level overrides into a single config passed to `axicli`.
- The node-pinned pod spawns `axicli` for the Pass; Job/Pass status is queryable while it runs and reflects the terminal outcome.
- A failed Pass marks the Job `failed`; retrying re-runs the Pass fresh (no resume logic yet — that's axicontrol-pause-resume).
- Submission UI: an `html/template` page with an htmx form to pick an uploaded file + Preset (with optional override fields) and submit the Job; a status view showing current Job/Pass state.

## Acceptance criteria

- [ ] Submitting a Job with a valid SVG + Preset produces a real plot on the AxiDraw
- [ ] Job/Pass status transitions `queued`/`pending` → `printing`/`running` → `complete` are observable via the API while the plot runs
- [ ] A Pass-level override (e.g. a different `speed_pendown`) actually changes plot behavior without needing a new Preset
- [ ] A forced CLI failure (e.g. disconnect the device mid-plot) results in Job `failed`, and resubmitting runs the Pass from scratch

## Blocked by

- axicontrol-upload-library
- axicontrol-device-config-presets
