---
name: axicontrol-architecture-spec-01-device-access
description: How the k8s-deployed service gets physical access to the USB-attached AxiDraw plotter
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

How does a k8s-deployed axicontrol service get physical access to the USB-attached AxiDraw plotter? Cover: whether pods need node affinity/pinning to the machine with the USB device, what k8s primitive exposes the device (hostPath, device plugin, privileged container, DaemonSet), and how this interacts with running the axidraw CLI as a subprocess.

## Execution Report

**Date:** 2026-07-31

### Decision

The cluster is multi-node, and there's a single AxiDraw unit permanently attached to one known machine. Device access is achieved by:

1. **Node pinning**: label that machine's node (e.g. `axidraw-device=present`) and use `nodeSelector`/`nodeAffinity` on the pod spec so it is only ever scheduled there.
2. **Stable device path**: a udev rule on that node matches the AxiDraw's USB vendor/product ID and creates a fixed symlink (e.g. `/dev/axidraw`), so the exposed path survives reconnects/reboots regardless of which `/dev/ttyACM<N>` the kernel happens to assign.
3. **Device exposure**: a `hostPath` volume mounts `/dev/axidraw` into the pod. No `privileged` container and no custom device plugin — those solve problems (broad `/dev` access, multiple movable devices) this setup doesn't have.
4. **Interaction with the CLI**: per the [AxiDraw CLI docs](https://axidraw.com/doc/cli_api/#introduction), `axicli` is stateless — a fresh subprocess per invocation, no persistent daemon, nothing retained between calls, auto-detecting the device over USB (or pinnable via `--port` if ever more than one unit exists). This means the node-pinned pod owns all job state (queued/printing/paused/resumed/etc.) itself, re-invoking the CLI per operation/layer rather than relying on it to track progress.

### Consequence

Whatever component talks to the AxiDraw CLI must run inside this node-pinned pod — that constrains the answer to [service shape](./axicontrol-architecture-spec-07-service-shape.md) ("Whether axicontrol is one backend or split into a device controller plus a web API"), which is now unblocked.
