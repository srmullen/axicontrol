# Checkpoint files share the FileStore, with their own key namespace and terminal-state deletion

Pass checkpoint files (the `axicli -o` output SVGs that hold resume progress) share the same `FileStore` and PersistentVolume adapter as uploads ([ADR-0005](./0005-filestore-abstraction-over-persistent-volume.md)) — both are SVGs only ever touched by `axicli` in the node-pinned pod, so there's no reason to decide storage twice. They diverge from uploads in two narrow, mechanical ways:

**Keying**: checkpoints live under their own `checkpoints/<pass-id>.svg` namespace, distinct from uploads' key scheme. A Pass's id is already unique and stable for its lifetime, so keying on it directly needs no extra id-minting, and the `checkpoints/` prefix makes the namespaces non-colliding by construction — including for `layers` Jobs, where each Pass reads the same source upload but writes to its own checkpoint key.

**Write pattern**: `axicli` is invoked with the same `-o checkpoints/<pass-id>.svg` for every invocation of a Pass, including its first never-paused run, overwriting the prior checkpoint in place on each pause. The CLI docs also show a chained-file pattern (`temp.svg` → `temp2.svg` → ...) for preserving intermediate checkpoints, but nothing here needs that — only the *latest* resume point ever matters, so a single stable key avoids both a versioning scheme and cleanup of stale intermediates.

**Retention**: unlike uploads' indefinite retention, `axicontrol` deletes a Pass's checkpoint as soon as that Pass reaches a terminal state (`complete`/`failed`/`cancelled`, delete-if-exists since a `failed` Pass may never have written one). A checkpoint is derived, purpose-built resume data, not a design asset — once resume is no longer possible there's nothing worth keeping.
