---
name: axicontrol-architecture-spec-06-notifications
description: What transport notifies a user when a plot completes, fails, or pauses
lane: done
tags: wayfinder:grilling
created-at: "2026-07-31"
created-by: seanmullen
assigned-to: seanmullen
parent: axicontrol-architecture-spec
---

## Question

What transport notifies a user when a plot completes, fails, or pauses? Decide among webhook, websocket/push, email, polling, or a combination, and where that hook lives in the service architecture.

## Execution Report

**Date:** 2026-07-31

### Decision

Two channels, serving two different situations:

**Out-of-band: generic outbound webhook.** axicontrol POSTs a JSON payload to one or more user-configured URLs, rather than building native integrations for specific providers (email, a particular push service). Any downstream system that accepts a webhook — ntfy, Discord, Home Assistant, IFTTT, etc. — becomes reachable without axicontrol maintaining provider-specific code. Fires on three transitions, chosen because they're the ones that need attention while unattended: a Job reaching `complete`, a Pass reaching `failed`, and a Job reaching `awaiting-next-pass` (per the [plot job data model](./axicontrol-architecture-spec-02-plot-job-data-model.md) — typically a pen-change point). User-initiated transitions (pause/resume/cancel) don't fire it — they're not surprising, the user just triggered them.

**Live: Server-Sent Events.** The API exposes an SSE stream of Job/Pass state changes for clients connected while a plot runs, taps the same underlying state-transition events as the webhook. Chosen over polling for lower latency at effectively no extra cost, and over websockets since this is purely server-to-client.

**Where the hook lives**: the node-pinned pod ([device access](./axicontrol-architecture-spec-01-device-access.md)) already owns Job/Pass state transitions as the thing driving them — it's the natural place to both emit the SSE stream and fire the webhook POST, rather than introducing a separate notification service.

### Consequence

Webhook URL configuration needs a place to persist, alongside Job/Pass/Preset/Device Config — no new ticket, already covered by [primary datastore](./axicontrol-architecture-spec-09-primary-datastore.md).
