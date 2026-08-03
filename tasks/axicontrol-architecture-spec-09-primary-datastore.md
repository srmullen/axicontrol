---
name: axicontrol-architecture-spec-09-primary-datastore
description: What storage technology backs axicontrol's structured records (jobs, passes, presets, device config, etc.)
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

What storage technology backs axicontrol's structured records — Jobs, Passes, Presets, Device Config, and (pending their own tickets) notification subscriptions and checkpoint-file metadata? Cover: embedded (SQLite) vs. a separate database service, and whether it needs to survive the node-pinned pod being rescheduled given [device access](./axicontrol-architecture-spec-01-device-access.md)'s node-pinning constraint. Feeds [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md), [configuration model](./axicontrol-architecture-spec-03-configuration-model.md), and [checkpoint file persistence](./axicontrol-architecture-spec-08-checkpoint-persistence.md).

## Execution Report

**Date:** 2026-08-03

### Decision

**Technology — embedded SQLite, not a separate database service.** Jobs, Passes, Presets, Device Config, and (later) notification subscriptions and checkpoint-file metadata are only ever read or written by the single node-pinned pod — [device access](./axicontrol-architecture-spec-01-device-access.md) established node pinning, and [service shape](./axicontrol-architecture-spec-07-service-shape.md) already ruled out splitting that pod into multiple services. There is exactly one process that ever touches this data, so nothing here needs a networked database's concurrent-writer or multi-consumer capabilities. A separate DB service (e.g. Postgres as its own deployment) would add a network hop, a second thing to deploy and keep available, and an operational dependency with no requirement driving it. Embedded SQLite runs in-process, has trivial backup semantics (copy the file), and matches the same "no split for no benefit at this scale" logic that shaped service-shape.

**Persistence — shares the `FileStore`'s PersistentVolume, not a separate one.** [Device access](./axicontrol-architecture-spec-01-device-access.md) established that the pod is *permanently* node-pinned — there is exactly one physical node with the AxiDraw attached, so there is no "reschedule to a different node" scenario to design around. What does still need handling: the pod can be killed and recreated on that same node (crash, redeploy), so the SQLite file needs to survive pod lifecycle the same way the `FileStore`'s blob data does ([file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md)) — a PersistentVolume, not pod-local ephemeral disk. Rather than provisioning a second PV, the SQLite file lives at its own subpath within the same PV already mounted for the `FileStore` (e.g. `/data/db/axicontrol.sqlite` vs. `/data/files/...`) — one volume to provision and back up, with no isolation requirement (single pod, single node, single process) that a second PV would buy anything for.

### Consequence

None — this closes the last open blocker [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md), [configuration model](./axicontrol-architecture-spec-03-configuration-model.md), and [checkpoint file persistence](./axicontrol-architecture-spec-08-checkpoint-persistence.md) were waiting on. No new tickets surfaced.
