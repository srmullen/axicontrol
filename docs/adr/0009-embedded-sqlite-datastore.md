# Embedded SQLite as the primary datastore, sharing the FileStore's PersistentVolume

Jobs, Passes, Presets, Device Config, notification subscriptions, and checkpoint-file metadata are structured records only ever read or written by the single node-pinned pod ([ADR-0001](./0001-node-pinned-device-access.md), [ADR-0007](./0007-single-backend-service.md)). Since exactly one process ever touches this data, none of it needs a networked database's concurrent-writer or multi-consumer capabilities — we chose embedded SQLite over a separate database service (e.g. Postgres as its own deployment), which would add a network hop, a second thing to deploy and keep available, and an operational dependency with no requirement driving it. SQLite runs in-process and has trivial backup semantics (copy the file).

The pod is *permanently* node-pinned, so there's no "reschedule to a different node" scenario to design around — but it can still be killed and recreated on that same node (crash, redeploy), so the SQLite file needs to survive pod lifecycle the same way the `FileStore`'s blob data does. Rather than provisioning a second PV, the SQLite file lives at its own subpath within the same PV already mounted for the `FileStore` (e.g. `/data/db/axicontrol.sqlite` vs. `/data/files/...`) — one volume to provision and back up, with no isolation requirement a second PV would buy anything for.

## Considered Options

- Separate database service (e.g. Postgres) — rejected: no concurrent writers or multiple consumers exist to justify the network hop and extra deployment.
- A second PersistentVolume dedicated to the SQLite file — rejected: no isolation need at single-pod, single-node, single-process scale.
