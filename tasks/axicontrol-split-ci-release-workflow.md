---
name: axicontrol-split-ci-release-workflow
description: Split GitHub Actions CI into a test/lint/build workflow on push-to-main and a release-published publish workflow to GHCR
lane: review
tags: ready-for-agent
created-at: "2026-08-07"
created-by: seanmullen
assigned-to: claude
depends-on: axicontrol-ci-pipeline
---

## Parent

Derived from a `/grill-with-docs` session. See [ADR-0015](../docs/adr/0015-split-ci-release-workflows.md) (split CI into test and release workflows; publish only on GitHub Release, drop `latest` tag), which supersedes part of [ADR-0013](../docs/adr/0013-build-ci-and-publish.md).

## Problem Statement

Today, `.github/workflows/ci.yml` is a single workflow that runs on every push to `main` *and* on `v*` tag pushes, and it always calls the Dagger `Publish` function — meaning every push to `main` publishes an image tagged `latest` to GHCR, whether or not anyone intended to ship anything. There's no separate signal for "just check my code" versus "cut a release." A developer pushing routine work to `main` gets a registry publish as a side effect they didn't ask for, and the homelab repo has no way to pin to a specific, deliberately-cut version — only to `latest`, a tag that silently moves underneath it with every merge.

## Solution

Split CI into two independent GitHub Actions workflows. A `ci.yml` workflow runs lint, test, and image build on every push to `main`, giving fast feedback with no registry interaction at all. A new `release.yml` workflow runs only when a GitHub Release is published (including pre-releases), re-runs the same lint/test/build gate, and only then pushes the image to GHCR under the release's own tag. Publishing becomes an explicit, deliberate action — authoring a GitHub Release — rather than an automatic side effect of merging to `main`. The floating `latest` tag is retired entirely; every image in GHCR corresponds to a real, named release.

## User Stories

1. As a developer pushing routine changes to `main`, I want CI to lint/test/build my change without publishing anything, so that I don't accidentally ship an image just by merging.
2. As a developer, I want to see lint and test failures on `main` quickly, so that I can fix regressions before they accumulate.
3. As a developer, I want the push-to-`main` workflow to also build the container image (not just lint/test), so that a broken Dockerfile is caught immediately rather than only when I try to cut a release.
4. As a release manager, I want publishing to GHCR to happen only when I create a GitHub Release, so that every published image corresponds to a deliberate, human-reviewed decision.
5. As a release manager, I want the release workflow to re-run lint, test, and build before publishing — even though `main` already passed those checks — so that a release cut from an older or re-tagged commit can't skip verification.
6. As a release manager, I want to be able to publish a pre-release (e.g. `v0.2.0-rc1`) the same way as a full release, so that I can hand the homelab repo a testable release candidate under its own explicit tag.
7. As a release manager, I want the published image tagged to exactly match the GitHub Release's tag, so that there's no ambiguity about which image a given release contains.
8. As a maintainer of the homelab repo (image consumer), I want to pin to a specific, named version tag rather than `latest`, so that my deployment doesn't change unexpectedly when someone pushes to `axicontrol`'s `main`.
9. As a maintainer of the homelab repo, I want `latest` to no longer be published at all, so that I'm not tempted to depend on a tag with no stable meaning.
10. As a repo maintainer, I want the push-to-`main` workflow to run with only read permissions (`contents: read`), so that it has no ability to write to the package registry even if compromised or misconfigured.
11. As a repo maintainer, I want the release workflow to hold `packages: write` permission only where it's actually needed (the workflow that publishes), so that registry-write access isn't granted more broadly than necessary.
12. As a repo maintainer, I want pushing a `v*` git tag alone (with no GitHub Release created) to trigger nothing, so that tagging for local/testing purposes can't accidentally publish an image.
13. As a repo maintainer, I want both workflows to keep using the existing Dagger module functions (`CI` and `Publish`) rather than duplicating pipeline logic in GitHub Actions YAML, so that the pipeline stays portable and runnable locally, per [ADR-0013](../docs/adr/0013-build-ci-and-publish.md).
14. As a repo maintainer, I want the new/changed workflow YAML to be syntactically validated (e.g. via `actionlint`), so that trigger and permission mistakes are caught before relying on a live GitHub Actions run.

## Implementation Decisions

- **Two workflow files.** `.github/workflows/ci.yml` is retargeted to run only Dagger's `CI` function (`Lint` → `Test` → `Build`, no publish) on `push` to `main`. A new `.github/workflows/release.yml` runs Dagger's `Publish` function (which internally still runs `CI` as a gate, then pushes to GHCR) on the `release` event with `types: [published]` — this fires for both full releases and pre-releases, with no filtering by `prerelease`.
- **No pull_request trigger.** `ci.yml` triggers on `push` to `main` only; PR-triggered checks are explicitly out of scope for this change.
- **No more tag-push trigger.** The existing `tags: ["v*"]` push trigger is removed. Pushing a version tag without creating a GitHub Release triggers nothing in either workflow.
- **`latest` tag retired.** `release.yml` never runs on a plain push to `main`, so the `Publish` function is never invoked with a non-tag `gitRef` in practice. `imageTag()` in `ci/main.go` is left unchanged (including its `"latest"` fallback branch and existing table test in `ci/main_test.go`) — the branch becomes unreachable in the new trigger topology, but removing it isn't part of this change's scope.
- **Permissions scoped per workflow.** `ci.yml` requests `contents: read` only. `release.yml` requests `contents: read` and `packages: write` (needed to push to GHCR).
- **No new Dagger functions.** Both workflows call existing Dagger module functions (`CI`, `Publish`) unchanged — the split is entirely at the GitHub Actions trigger/workflow level, not the pipeline logic level, preserving the "GitHub Actions as thin trigger" principle from [ADR-0013](../docs/adr/0013-build-ci-and-publish.md).
- **`git-ref` plumbing unchanged.** `release.yml` passes `${{ github.ref }}` through to `Publish` exactly as `ci.yml` does today; for a `release: published` event, GitHub Actions sets `github.ref` to the release's tag ref (e.g. `refs/tags/v1.2.3`), which `imageTag()` already handles correctly.
- **Documentation:** [ADR-0015](../docs/adr/0015-split-ci-release-workflows.md) records this decision and its rejected alternatives; [ADR-0013](../docs/adr/0013-build-ci-and-publish.md) is annotated with a "superseded in part" note pointing to it, following the existing pattern used for [ADR-0014](../docs/adr/0014-python-base-image-for-axicli.md).

## Testing Decisions

- This is CI/workflow configuration, not application code — there is no new Go seam to unit test. `ci/main.go`'s `CI` and `Publish` functions, and `imageTag()`, are unchanged and already covered by `ci/main_test.go`.
- Workflow YAML correctness (trigger conditions, permissions blocks, valid syntax) should be checked with `actionlint .github/workflows/ci.yml .github/workflows/release.yml`, the same tool used to validate `ci.yml` when it was first built (see `axicontrol-ci-pipeline`'s execution report).
- Local pipeline behavior can still be exercised the same way as before via `dagger call -m ./ci ci --source=.` and `dagger call -m ./ci publish --source=. ...` — no change needed there since the Dagger functions themselves aren't changing.
- **Not verifiable in an agent session:** actually triggering `release.yml` via a live GitHub Release, or `ci.yml` via a live push to `main`, and confirming the resulting GHCR image and tag — this requires real GitHub Actions execution with repo secrets, the same limitation flagged in the original `axicontrol-ci-pipeline` ticket's execution report. Flag this explicitly as not-live-verified rather than claiming it works end-to-end.

## Out of Scope

- Removing or refactoring `imageTag()`'s now-unreachable `"latest"` fallback branch, or its associated tests.
- Adding a `pull_request` trigger to `ci.yml` (explicitly decided against).
- Any mechanism for publishing a `main`/dev-build image (e.g. under a `main` or `edge` tag) for staging/testing purposes — if that's needed later, it's a separate, explicit trigger to design.
- Release automation (e.g. auto-generating releases from conventional commits, goreleaser, release-please) — releases are still created manually via the GitHub UI/CLI; this ticket only wires up what happens once one is published.
- Any change to `docs/adr/0014-python-base-image-for-axicli.md` or the Dockerfile/image contents themselves.
- Branch protection / required-status-check configuration in GitHub repo settings.

## Further Notes

- No tags or GitHub Releases exist in this repo yet, so this design is greenfield — there's no existing release process or consumer behavior to migrate off of.
- The homelab repo (separate repo, out of scope here) currently has no documented dependency on the `latest` tag being verified, but should be checked/updated if it does reference `ghcr.io/<owner>/axicontrol:latest` anywhere, since that tag will stop being published.

## Execution Report

**Date:** 2026-08-07

- `.github/workflows/ci.yml`: removed the `tags: ["v*"]` push trigger and `packages: write` permission; job renamed `publish` → `ci`; now calls Dagger's `ci` function (`--source=.`) instead of `publish`, so it lints/tests/builds on every push to `main` with no registry interaction.
- `.github/workflows/release.yml` (new): triggers on `release: types: [published]` (covers pre-releases too, no filtering); `contents: read` + `packages: write`; calls Dagger's `publish` function unchanged from the old `ci.yml` invocation (`--source`, `--owner`, `--registry-username`, `--registry-password`, `--git-ref`).
- No changes to `ci/main.go` — `CI`, `Publish`, and `imageTag()` are untouched, matching the PRD's testing-seam decision to reuse the existing Dagger function boundary rather than add new pipeline logic.

**Verified locally, this session:**
- `go test ./...` — full app test suite passes.
- `dagger run go test ./...` (inside `ci/`) — `imageTag` table test still passes unchanged.
- `actionlint .github/workflows/ci.yml .github/workflows/release.yml` — zero issues.
- `dagger call -m ./ci ci --source=.` — full lint→test→build chain succeeds locally, exactly what `ci.yml` now invokes.
- `dagger call -m ./ci publish --help` — confirmed the `publish` function's argument signature is unchanged and matches `release.yml`'s call exactly.

**Not verified — needs a live GitHub Actions run:** an actual push to `main` triggering `ci.yml`, or an actual GitHub Release triggering `release.yml` and landing a correctly-tagged image in GHCR. Same limitation as the original `axicontrol-ci-pipeline` ticket — requires repo secrets in a live Actions run, which this session can't exercise. The pipeline logic itself is exercised end-to-end locally via `dagger call`.
