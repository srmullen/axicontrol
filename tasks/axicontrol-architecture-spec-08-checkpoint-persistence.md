---
name: axicontrol-architecture-spec-08-checkpoint-persistence
description: Where Pass checkpoint files (axicli resume output SVGs) are persisted and whether they survive pod restarts
lane: backlog
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
parent: axicontrol-architecture-spec
---

## Question

Where do Pass checkpoint files persist — the `axicli -o` output SVGs that hold resume progress for a paused Pass — and do they need to survive the node-pinned pod restarting or being rescheduled (e.g. local pod disk vs. a PersistentVolume vs. the same storage backend chosen for uploaded files)? See [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md) for how Passes and checkpoints fit into the Job model, and [device access](./axicontrol-architecture-spec-01-device-access.md) for the node-pinned pod they run in.

**Narrowed by [file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md):** that ticket already decided checkpoint files share the same `FileStore` abstraction and PersistentVolume adapter as uploaded files — both are SVGs only ever touched by `axicli` in the node-pinned pod. What's left here is much smaller: confirm that framing covers checkpoints cleanly (e.g. naming/keying convention so a Pass's checkpoint doesn't collide with its source upload in the same store), or flag anything about checkpoints specifically that doesn't fit the uploaded-file model.
