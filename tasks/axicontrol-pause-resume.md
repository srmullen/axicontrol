---
name: axicontrol-pause-resume
description: Pause and resume a running plot via checkpoint files
lane: backlog
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-print-whole-job
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0002](../docs/adr/0002-job-pass-data-model.md) (pause/resume transitions) and [ADR-0008](../docs/adr/0008-checkpoint-file-persistence.md) (checkpoint files).

## What to build

Pause and resume a running Pass, end to end, on the real device.

- Pause: pod sends SIGINT to the running `axicli` subprocess; it finishes its current line segment, writes its checkpoint, exits. Pass → `paused`.
- Every `axicli` invocation for a Pass (including its first) uses `-o checkpoints/<pass-id>.svg` in the `FileStore`, so pausing always has a checkpoint to write to.
- Resume: pod re-invokes `axicli` with `res_plot`/`res_home` against that same checkpoint key. Pass → `running`.
- Checkpoint retention: delete `checkpoints/<pass-id>.svg` as soon as the Pass reaches a terminal state (`complete`/`failed`/`cancelled`); delete-if-exists, since a `failed` Pass may not have left one.
- Cancel: from `pending`/`paused`/`running`, Pass and Job → `cancelled` (terminal).

## Acceptance criteria

- [ ] Pausing a running Pass stops the physical plot cleanly (no mid-line jump) and the Job reflects `paused`
- [ ] Resuming continues the same physical plot from where it paused, not from the beginning
- [ ] A Pass can be paused and resumed more than once
- [ ] The checkpoint file is gone from the `FileStore` once the Pass reaches `complete`, `failed`, or `cancelled`
- [ ] Cancelling a paused Pass leaves no checkpoint behind and the Job is `cancelled`

## Blocked by

- axicontrol-print-whole-job
