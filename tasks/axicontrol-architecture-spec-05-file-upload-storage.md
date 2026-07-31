---
name: axicontrol-architecture-spec-05-file-upload-storage
description: How plot files are uploaded, stored, and retained
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

How are plot files uploaded and stored? Decide accepted file formats (SVG only, or others the AxiDraw CLI accepts), the storage backend, the upload flow, and the retention/lifecycle of uploaded files.

## Execution Report

**Date:** 2026-07-31

### Decision

**Format**: SVG exclusively — confirmed against the [AxiDraw CLI docs](https://axidraw.com/doc/cli_api/), which accept nothing else. Not really a decision, just a constraint: no format negotiation needed.

**Storage backend**: a `FileStore` abstraction (put/get/delete behind an interface) rather than coupling the rest of the system to one storage technology. The initial adapter is a PersistentVolume mounted into the node-pinned pod ([device access](./axicontrol-architecture-spec-01-device-access.md)) — `axicli` needs a real local filesystem path regardless, so this avoids a download/upload dance for no benefit at current scale. The abstraction leaves room for a future object-storage adapter (e.g. S3-compatible) without touching calling code. This same `FileStore` backs both uploaded files (this ticket) and Pass checkpoint files ([checkpoint file persistence](./axicontrol-architecture-spec-08-checkpoint-persistence.md)) — both are SVGs only ever touched by `axicli` in the node-pinned pod, so there's no reason to decide storage twice.

**Upload flow**: standard HTTP multipart upload. On receipt, the SVG is sanitized (strip embedded AxiDraw layer option overrides, per [configuration model](./axicontrol-architecture-spec-03-configuration-model.md)) before being written to the `FileStore`. Whichever component actually serves the upload endpoint must end up writing through this same `FileStore` instance — if [service shape](./axicontrol-architecture-spec-07-service-shape.md) ends up splitting a web-API pod from the device-pinned pod, uploads have to be proxied to the device-pinned pod's internal API rather than sharing the PV directly across pods on different nodes. Noted as an input to that still-open ticket, not resolved here.

**Retention**: indefinite. Uploaded SVGs function as a personal library of designs — any of them can be the source file for a new Job later, not just the one they were originally uploaded for. Deletion is an explicit user action, never automatic; storage cost is a non-issue at this scale.

### Consequence

Feeds [checkpoint file persistence](./axicontrol-architecture-spec-08-checkpoint-persistence.md) directly — that ticket can now just confirm "same `FileStore`, PersistentVolume adapter" rather than re-deciding storage technology from scratch.
