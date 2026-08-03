# Job/Pass data model mapped one-to-one onto axicli invocations

A plot Job has an ordered list of Passes, where each Pass corresponds to exactly one `axicli` invocation: a `whole`-mode Job has a single Pass; a `layers`-mode Job has one Pass per layer number, auto-discovered by parsing the SVG's layer names for AxiDraw's numeric-prefix convention. Job status (`queued → printing → (paused ⇄ printing) → awaiting-next-pass → complete`, or `failed`/`cancelled`) is derived from its Passes' statuses, not tracked independently.

We deliberately made advancing to the next Pass require an explicit user trigger rather than auto-advancing — layers are typically used for manual pen changes, so silently continuing to the next color would be wrong more often than right. Pause is implemented as SIGINT plus a CLI-written checkpoint file (see [ADR-0008](./0008-checkpoint-file-persistence.md)); resume re-invokes `axicli` with `res_plot`/`res_home` against that file rather than the pod tracking in-progress plot state itself, since `axicli` gives us no other way to resume mid-plot.

## Consequences

A Pass that fails may not have left a resumable checkpoint (a crash isn't a clean pause), so retry always re-runs the Pass fresh rather than assuming resumability.
