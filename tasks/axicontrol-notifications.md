---
name: axicontrol-notifications
description: Outbound webhook and SSE notifications for plot status
lane: review
tags: ready-for-agent, user-management
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-print-whole-job
---

## Parent

Derived from the [axicontrol-architecture-spec](./axicontrol-architecture-spec.md) map. See [ADR-0006](../docs/adr/0006-webhook-and-sse-notifications.md) (webhook + SSE), [ADR-0011](../docs/adr/0011-backend-stack.md) (stdlib `net/http` client for delivery), and [ADR-0012](../docs/adr/0012-frontend-stack.md) (htmx SSE extension for live UI updates).

## What to build

Two notification channels, both driven off the Job/Pass state transitions established in axicontrol-print-whole-job (and, once merged, axicontrol-layers-mode's `awaiting-next-pass`).

- Webhook config: user can register one or more destination URLs, persisted in the datastore.
- Webhook firing: a JSON POST (stdlib `net/http` client, no external library) to each configured URL fires on a Job reaching `complete`, a Pass reaching `failed`, and a Job reaching `awaiting-next-pass`. User-initiated transitions (pause/resume/cancel) do not fire it.
- SSE: an endpoint streams Job/Pass state changes live to connected clients, sourced from the same transition events as the webhook; the Job status page consumes it via htmx's SSE extension so status updates without polling or a page reload.

## Acceptance criteria

- [ ] Registering a webhook URL and completing a Job results in a POST to that URL with the Job's final state
- [ ] A Pass failure fires the webhook; a user-initiated pause/resume/cancel does not
- [ ] A client connected to the SSE endpoint during a running plot receives status updates without polling
- [ ] Multiple registered webhook URLs all receive the same event

## Blocked by

- axicontrol-print-whole-job
