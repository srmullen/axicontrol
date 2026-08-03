# Single backend service, not a device-controller/web-API split

axicontrol is one backend service. The node-pinned pod ([ADR-0001](./0001-node-pinned-device-access.md)) serves the entire HTTP API directly — Job/Pass/Preset/Device Config CRUD, uploads through the `FileStore`, the hardware self-test/jog endpoints, and the SSE stream and webhook firing — in one process/deployment. Every other decision already required that pod for its core operation, so splitting out a stateless web-API service would add a network boundary and service-to-service auth to build, in exchange for independent deploys and a path to multiple devices — benefits that don't apply at this system's actual scale of a single user and a single device.

## Considered Options

- Device controller (hardware-pinned) + stateless web API, communicating over the network — rejected: no requirement it serves at this scale, and it complicates every other decision (storage, notifications, uploads) that's simplest when everything lives in one pod.
