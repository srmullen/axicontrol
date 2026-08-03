# Node-pinned pod with udev-stabilized hostPath for AxiDraw access

The cluster is multi-node but there is a single AxiDraw permanently attached to one machine. We label that node (`axidraw-device=present`) and use `nodeSelector`/`nodeAffinity` so the pod is only ever scheduled there, expose the device via a `hostPath` mount of a udev-stabilized `/dev/axidraw` symlink (so the path survives reconnects/reboots regardless of the kernel-assigned `/dev/ttyACM<N>`), and invoke `axicli` as a subprocess from within that pod.

We chose plain `hostPath` over a custom device plugin or a `privileged` container: both of those solve problems this setup doesn't have (broad `/dev` access, multiple movable devices across nodes). Because `axicli` is stateless per invocation (no persistent daemon, nothing retained between calls), the node-pinned pod itself must own all job state — this decision is the load-bearing constraint behind [ADR-0007](./0007-single-backend-service.md) (the whole backend collapses to this one pod) and behind persistence living on a PV local to that node ([ADR-0005](./0005-filestore-abstraction-over-persistent-volume.md), [ADR-0009](./0009-embedded-sqlite-datastore.md)).

## Considered Options

- Device plugin — rejected, built for multiple/dynamic devices; overkill for one fixed unit.
- Privileged container with broad `/dev` access — rejected, wider blast radius than a single stable device path needs.
