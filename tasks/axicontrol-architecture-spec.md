---
name: axicontrol-architecture-spec
description: System-level architecture spec for axicontrol, ready to hand off for implementation
lane: backlog
tags: wayfinder:map
created-at: "2026-07-31"
created-by: seanmullen
---

## Destination

A system-level architecture spec for axicontrol: the data model, services, and API that wrap the AxiDraw CLI, plus the deployment shape for running in k8s — ready to hand off to implementation. Excludes UI/UX design (layout, interaction flow) and speculative "what else could we build on top" features (README's open question).

## Notes

- Consult `docs/agents/issue-tracker.md` for how this map's tickets are expressed in taipir (lanes, tags, blocking via `depends-on`).
- Reference the [AxiDraw CLI docs](https://axidraw.com/doc/cli_api/) for the capabilities being wrapped.
- Default ticket type is `grilling`; use `research` for external lookups (k8s device-access patterns, AxiDraw CLI capabilities) and `prototype` only if a concrete artifact is needed to react to.

## Decisions so far

## Not yet specified

- How "view the file that will be plotted" resolves at the system level (raw file serving vs. server-side preview rendering) — depends on the service-shape and file-storage decisions below.

## Out of scope

- UI/UX design (layout, interaction flow) — left for a later prototyping effort.
- "What else could be built on top of axidraw control" (README's open question) — left for a future effort once the core exists.
