---
name: axicontrol-architecture-spec-09-primary-datastore
description: What storage technology backs axicontrol's structured records (jobs, passes, presets, device config, etc.)
lane: backlog
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
parent: axicontrol-architecture-spec
---

## Question

What storage technology backs axicontrol's structured records — Jobs, Passes, Presets, Device Config, and (pending their own tickets) notification subscriptions and checkpoint-file metadata? Cover: embedded (SQLite) vs. a separate database service, and whether it needs to survive the node-pinned pod being rescheduled given [device access](./axicontrol-architecture-spec-01-device-access.md)'s node-pinning constraint. Feeds [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md), [configuration model](./axicontrol-architecture-spec-03-configuration-model.md), and [checkpoint file persistence](./axicontrol-architecture-spec-08-checkpoint-persistence.md).
