---
name: axicontrol-architecture-spec-07-service-shape
description: Whether axicontrol is one backend or split into a device controller plus a web API
lane: backlog
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
parent: axicontrol-architecture-spec
depends-on: axicontrol-architecture-spec-01-device-access
---

## Question

Is axicontrol one backend service, or split into a hardware-pinned "device controller" plus a stateless web API? Decide the service boundary given the device-access answer from [device access](./axicontrol-architecture-spec-01-device-access.md), and how the pieces would communicate if split.
