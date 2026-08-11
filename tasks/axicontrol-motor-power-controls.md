---
name: axicontrol-motor-power-controls
description: Expose disable_xy/enable_xy manual commands in the jog panel, with motor-state guarding
lane: done
tags: ready-for-agent
created-at: "2026-08-10"
created-by: seanmullen
assigned-to: claude
parent: axicontrol-testing-panel
related: axicontrol-testing-panel
---

## Parent

Follow-on to [axicontrol-testing-panel](./axicontrol-testing-panel.md). See [ADR-0004](../docs/adr/0004-carriage-position-tracking.md) (carriage position tracking) — it already names "motors enabled" as a valid position-zeroing reference point alongside `walk_home`, but no UI action currently triggers that case.

## What to build

Expose axicli's `--mode manual --manual_cmd disable_xy` / `enable_xy` pair in the jog panel (`internal/api/testing.go`, `templates/testing.html`), plus the state tracking and guarding this pair implies:

- `POST /testing/disable-xy` (`handleTestDisableXY`) → `manualCmdArgs("disable_xy", devicePath)`. On success, sets a new in-memory `motorsDisabled bool` (guarded by the existing `posMu`, alongside `carriageX/Y`). No position change.
- `POST /testing/enable-xy` (`handleTestEnableXY`) → `manualCmdArgs("enable_xy", devicePath)`. On success, clears `motorsDisabled` **and** zeroes `carriageX/Y` — per ADR-0004, "motors enabled" is a zero-reference event just like `walk_home`.
- `handleTestAlign`'s success path also sets `motorsDisabled = true`: `--mode align` de-energizes the XY motors too (it raises the pen and turns off the XY steppers so the carriage can be moved by hand), so it's another motors-off entry point, not an independent state.
- `motorsDisabled` defaults to `true` on server start, matching the hardware's actual power-up-off state.
- While `motorsDisabled` is true, `handleTestMove` and `handleTestWalkHome` reject server-side before claiming the device or invoking axicli, re-rendering the panel with a friendly inline message (same pattern as `deviceBusyMessage`), e.g. `"motors are disabled — click \"Enable motors\" first"`. This is a hard server-side guard, not just a disabled HTML attribute, since a jog action reordering carriage state incorrectly is the failure mode being prevented (same reasoning as `tryClaimDevice`'s existing hard guard).
- `handleTestCycle`/`handleTestToggle` are **not** guarded — `cycle`/`toggle` only operate the pen-lift servo and are unaffected by XY motor state.
- Template: a new button row above the existing Cycle/Toggle/Align/Home row, with "Disable motors" / "Enable motors" buttons. Move/Home buttons/form rendered with the HTML `disabled` attribute when `motorsDisabled` is true, to match the server-side guard. A warning line shown whenever `motorsDisabled` is true, e.g. `"Motors disabled — carriage can be moved by hand; position unknown until re-enabled."`

## Acceptance criteria

- [ ] `disable-xy`/`enable-xy` buttons trigger the corresponding axicli manual commands and reflect real device behavior
- [ ] A successful Enable zeroes tracked carriage position, same as `walk_home`
- [ ] A successful Align also flips `motorsDisabled` (verify via a subsequent guarded Move/Home rejection)
- [ ] Move-to-coordinate and Home are rejected server-side (no axicli invocation) while `motorsDisabled` is true, with an inline error shown
- [ ] Cycle/Toggle remain usable regardless of `motorsDisabled` state
- [ ] Fresh server start has `motorsDisabled = true` (Move/Home blocked until an explicit Enable)

## Blocked by

- axicontrol-testing-panel

## Execution Report

**Date:** 2026-08-10

Built per spec: `handleTestDisableXY`/`handleTestEnableXY` (`internal/api/testing.go`), routed at `POST /testing/disable-xy`/`POST /testing/enable-xy`. `motorsDisabled bool` added to `Server`, guarded by the existing `posMu` alongside `carriageX/Y`, defaulting to `true` in `NewServer` (matches hardware power-up-off state). `handleTestAlign` now also flips it true on success (align de-energizes the XY motors same as disable_xy). `handleTestEnableXY` clears it and zeroes tracked position in the same locked section, matching ADR-0004's "motors enabled is a zero-reference event" language. `requireMotorsEnabled` guards `handleTestMove`/`handleTestWalkHome` server-side, before device claim or any axicli call, rendering `motorsDisabledMessage` inline instead. Cycle/Toggle deliberately left unguarded (pen-lift servo only, verified against the axicli docs that XY motor state doesn't affect them). Template gained a toggling Disable/Enable button, a warning line shown while disabled, and `disabled` attributes on the Move/Home controls to match the server-side guard.

Went through `/grilling` before implementation to pin down scope (disable_xy alone vs. the enable_xy pair — went with both, since ADR-0004 already named "motors enabled" as a zero-reference event with no UI path to trigger it), the stale-position warning, which actions to guard (narrowed from all four utility buttons down to just Move/Home once confirmed via the axicli docs that Cycle/Toggle don't touch the XY motors), guard enforcement (server-side, not just disabled HTML attributes), button placement, Align's overlap with Disable, and the default flag state on a fresh server (chose hardware-accurate `true` over preserving the old always-usable-immediately behavior).

Verified: `go build`/`go vet`/`golangci-lint run`/`go test -race ./...` all clean. Extensive coverage in `testing_test.go`: arg-building for both new endpoints, Align setting the flag, the guard rejecting Move/Home before reaching axicli (both on a fresh server and after an explicit Align), Cycle/Toggle staying usable regardless of motor state, Enable zeroing tracked position, Disable leaving it untouched, and a fresh-restart server defaulting back to disabled. All pre-existing Move/Home tests updated to call the new `enableMotors` test helper first, since they'd previously relied on motors being usable with no explicit enable step. Reaching a real AxiDraw is unverifiable here, same caveat prior tickets have recorded.

Ran `/code-review` (Standards + Spec axes) against the working-tree diff before committing. Standards review flagged `handleTestAlign`/`handleTestDisableXY` as byte-identical after `runTestAction` — extracted a shared `setMotorsDisabled(bool)` helper. Also noted `requireMotorsEnabled`'s naming reads slightly differently from `tryClaimDevice`'s pattern; judged not worth changing since it's a distinct shape (check + render-response combined, not a bare predicate) and the name is already unambiguous. Spec review flagged one coverage gap (no test asserting disable_xy leaves position untouched) — added `TestTestDisableXYLeavesTrackedPositionUnchanged`. It also flagged the toggling single button as a literal deviation from the ticket's "'Disable motors' / 'Enable motors' buttons" phrasing; kept the toggle deliberately (only ever show the one valid action, consistent with the server-side hard-guard philosophy already chosen for Move/Home) rather than showing both simultaneously.
