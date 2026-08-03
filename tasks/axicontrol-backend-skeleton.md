---
name: axicontrol-backend-skeleton
description: Device-pinned backend skeleton deployable to k8s with embedded SQLite
lane: backlog
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0001](../docs/adr/0001-node-pinned-device-access.md) (device access), [ADR-0007](../docs/adr/0007-single-backend-service.md) (single backend service), and [ADR-0009](../docs/adr/0009-embedded-sqlite-datastore.md) (embedded SQLite).

## What to build

A single backend service, deployed to k8s, node-pinned to the machine with the AxiDraw attached and able to reach it. This is the platform every other axicontrol feature runs inside — no UI, no Job/Pass model yet, just: the pod exists, it's scheduled correctly, it can see the device, and it has durable local storage.

- Label the AxiDraw-attached node; the pod spec uses `nodeSelector`/`nodeAffinity` so it's only ever scheduled there.
- A udev rule creates a stable `/dev/axidraw` symlink; a `hostPath` volume mounts it into the pod.
- A single HTTP service process runs in that pod, backed by a PersistentVolume.
- Embedded SQLite lives at its own subpath on that PV (e.g. `/data/db/axicontrol.sqlite`).
- A minimal endpoint proves the whole path works end to end: it shells out to `axicli sysinfo` and returns the result, and a second endpoint writes/reads a trivial row to confirm SQLite persists across a pod restart.

## Acceptance criteria

- [ ] Pod only schedules on the AxiDraw-attached node (verified by draining/cordoning the other node(s))
- [ ] Pod restart (delete pod, let it reschedule on the same node) does not lose SQLite data
- [ ] An endpoint successfully runs `axicli sysinfo` against the real device and returns its output
- [ ] `/dev/axidraw` symlink survives a device reconnect/reboot without the pod needing reconfiguration

## Blocked by

None - can start immediately
