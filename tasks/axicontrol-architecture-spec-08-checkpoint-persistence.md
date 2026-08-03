---
name: axicontrol-architecture-spec-08-checkpoint-persistence
description: Where Pass checkpoint files (axicli resume output SVGs) are persisted and whether they survive pod restarts
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

Where do Pass checkpoint files persist — the `axicli -o` output SVGs that hold resume progress for a paused Pass — and do they need to survive the node-pinned pod restarting or being rescheduled (e.g. local pod disk vs. a PersistentVolume vs. the same storage backend chosen for uploaded files)? See [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md) for how Passes and checkpoints fit into the Job model, and [device access](./axicontrol-architecture-spec-01-device-access.md) for the node-pinned pod they run in.

**Narrowed by [file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md):** that ticket already decided checkpoint files share the same `FileStore` abstraction and PersistentVolume adapter as uploaded files — both are SVGs only ever touched by `axicli` in the node-pinned pod. What's left here is much smaller: confirm that framing covers checkpoints cleanly (e.g. naming/keying convention so a Pass's checkpoint doesn't collide with its source upload in the same store), or flag anything about checkpoints specifically that doesn't fit the uploaded-file model.

## Execution Report

**Date:** 2026-08-03

### Decision

**Keying**: checkpoints get their own key namespace, distinct from uploads' — `checkpoints/<pass-id>.svg`, vs. whatever key scheme uploads use (e.g. `uploads/<file-id>.svg`). A Pass's id is already unique and stable for its lifetime ([plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md)), so keying directly on it needs no extra id-minting, and the `checkpoints/` prefix makes the two namespaces non-colliding by construction. For a `layers` Job, each Pass (one per layer) reads the same uploaded source SVG as input but writes to its own distinct checkpoint key — no collision, since the key is per-Pass, not per-Job or per-source-file.

**Write pattern — overwrite in place, one key per Pass**: the node-pinned pod always invokes `axicli` with `-o checkpoints/<pass-id>.svg`, for every invocation of that Pass — including its first, never-paused run. Confirmed against the [AxiDraw CLI docs](https://axidraw.com/doc/cli_api/): pausing writes checkpoint data into the `-o` output file, and reusing the same output path across repeated pause/resume cycles is explicitly supported — each pause simply overwrites the prior checkpoint data in that same file. The CLI's own docs illustrate an alternative chained-file pattern (`temp.svg` → `temp2.svg` → ...) for cases where intermediate checkpoints need preserving, but nothing here calls for that: a checkpoint only ever needs to hold the *latest* resume point, so a single stable key reused in place avoids minting a versioning scheme and avoids accumulating intermediate files that would need cleanup.

**Retention — diverges from uploads**: uploads are retained indefinitely as a personal design library ([file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md)); checkpoints are not. `axicontrol` deletes `checkpoints/<pass-id>.svg` from the `FileStore` as soon as that Pass reaches a terminal state (`complete`/`failed`/`cancelled`). A checkpoint is derived, purpose-built resume data — not a design asset — so once resume is no longer possible or needed there's nothing worth keeping, and unbounded accumulation of stale per-Pass files serves no purpose. (A `failed` Pass may not have left a checkpoint at all, per [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md)'s note that a crash doesn't necessarily leave a clean resumable checkpoint — deletion here is delete-if-exists, not a hard requirement.)

**Verdict**: the `FileStore` framing from file-upload-storage covers checkpoints cleanly. The two respects in which checkpoints diverge from uploads — a separate key namespace and a terminal-state deletion policy instead of indefinite retention — are both narrow, mechanical divergences within the same abstraction, not something that breaks the shared-`FileStore` model.

### Consequence

None — this closes the last open question the file-upload-storage decision deferred. No new tickets surfaced.
