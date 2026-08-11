---
name: axicontrol-device-busy-visibility
description: Any print page shows when the AxiDraw is busy with a different upload's job
lane: todo
tags: ready-for-agent
created-at: "2026-08-11"
created-by: seanmullen
depends-on: axicontrol-print-page-job-creation
---

## Parent

See [ADR-0017](../docs/adr/0017-per-upload-print-page-as-job-entry-point.md) and the `CONTEXT.md` glossary (Device-busy entry).

## What to build

Any print page — regardless of which Upload it belongs to — should proactively show when the AxiDraw is busy running a Job for a *different* Upload, instead of only rejecting a submission after the fact.

- The device-busy state already exists in-process (`deviceClaimed`, `internal/api/jobs_run.go`) but isn't exposed to any page. Expose which Upload/Job it's currently claimed by, and what it's doing (e.g. current layer/pass), to print pages other than the one that owns the running Job.
- On a print page whose own Upload isn't the busy one, show a banner/status reflecting this (e.g. "AxiDraw is busy: printing Layer 3/5 of sign.svg"), live-updating the same way the owning page's own status does.
- Blocking behavior itself does not change — new-Job submission is still rejected while `deviceClaimed` is true (existing behavior from `axicontrol-print-page-job-creation`); this ticket is purely about making that state visible ahead of a submit attempt, on every print page, not just the busy one.

## Acceptance criteria

- [ ] Opening upload B's print page while upload A has a Job running shows that the device is busy and what it's doing (which file, which layer/pass if applicable)
- [ ] That status updates live as the running Job progresses (e.g. layer/pass changes), without a manual reload
- [ ] Once the running Job finishes, the busy banner clears from other print pages
- [ ] Attempting to submit a new Job from a non-busy-owning print page while the device is busy is still rejected (unchanged from existing `deviceClaimed` behavior)

## Blocked by

- axicontrol-print-page-job-creation
