---
name: axicontrol-architecture-spec-07-service-shape
description: Whether axicontrol is one backend or split into a device controller plus a web API
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
depends-on: axicontrol-architecture-spec-01-device-access
---

## Question

Is axicontrol one backend service, or split into a hardware-pinned "device controller" plus a stateless web API? Decide the service boundary given the device-access answer from [device access](./axicontrol-architecture-spec-01-device-access.md), and how the pieces would communicate if split.

## Execution Report

**Date:** 2026-07-31

### Decision

Single backend service. The node-pinned pod ([device access](./axicontrol-architecture-spec-01-device-access.md)) serves the entire HTTP API directly — Job/Pass/Preset/Device Config CRUD, uploads through the `FileStore` ([file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md)), the hardware self-test/jog endpoints ([testing feature](./axicontrol-architecture-spec-04-testing-feature.md)), the SSE stream and webhook firing ([notifications](./axicontrol-architecture-spec-06-notifications.md)) — in one process/deployment. Every prior decision already required the node-pinned pod for its core operation; a separate web-API-plus-device-controller split would add a network boundary and service-to-service auth to build, in exchange for independent deploys and a path to multiple devices — benefits that don't apply at this system's actual scale (single user, single device).

### Consequence

Settles the last open half of the "view the file that will be plotted" fog entry — the file-storage half settled with [file upload & storage](./axicontrol-architecture-spec-05-file-upload-storage.md), and with a single backend, the same pod that holds the `FileStore` also serves the HTTP API, so raw file serving is straightforward. But it surfaces a sharper, previously-hidden question: uploaded SVGs can embed `<script>`/event-handler content, so serving them for direct browser rendering is a real XSS surface, not just a serving-mechanism choice. New ticket: [SVG preview safety](./axicontrol-architecture-spec-10-svg-preview-safety.md).
