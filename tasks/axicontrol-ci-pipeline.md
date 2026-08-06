---
name: axicontrol-ci-pipeline
description: 'Dagger-driven CI: lint, test, build, and publish the container image to GHCR'
lane: review
tags: ready-for-agent, user-management
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-backend-skeleton
---

## Parent

Derived from a `/grill-with-docs` tech-stack session. See [ADR-0013](../docs/adr/0013-build-ci-and-publish.md) (build, CI, and publish).

## What to build

Automated CI that lints, tests, builds, and publishes the container image from axicontrol-backend-skeleton — with the pipeline logic itself written portably (Dagger), not locked into GitHub-Actions-specific YAML.

- A Dagger module (Go SDK) defining the pipeline: `golangci-lint` → `go test` → `docker build` (the multi-stage Dockerfile from axicontrol-backend-skeleton — a Python-capable final image per [ADR-0014](../docs/adr/0014-python-base-image-for-axicli.md), not `distroless/static`) → publish.
- A thin GitHub Actions workflow that triggers the Dagger pipeline on push to `main` and on version tags — the workflow itself contains no build/test/lint logic, only "run the Dagger pipeline."
- Images published to GHCR (`ghcr.io/<owner>/axicontrol`): `latest` on `main`, a matching semver tag on version tags — so the homelab repo can pin to a specific tag.
- A lint or test failure stops the pipeline before it attempts to build or publish.

## Acceptance criteria

- [ ] Pushing to `main` runs lint, then test, then build; on success an image is published to `ghcr.io/<owner>/axicontrol:latest` — **not live-verified**, needs an actual push to `main` on GitHub with repo secrets available; the pipeline logic itself (`Publish` → `CI` → lint/test/build, `imageTag` deriving `latest` from a non-tag ref) is implemented and unit-tested
- [ ] Pushing a version tag (e.g. `v0.1.0`) publishes an image tagged to match — **not live-verified**, same reason; `imageTag("refs/tags/v0.1.0") == "v0.1.0"` is covered by `ci/main_test.go`
- [x] The same Dagger pipeline runs locally (e.g. `dagger call ...`) and produces the same lint/test/build outcome without GitHub Actions involved
- [x] A failing `golangci-lint` or `go test` run stops the pipeline before the build/publish steps run
- [x] The GitHub Actions workflow file contains no lint/test/build logic itself — only the trigger and the call into the Dagger pipeline

## Blocked by

- axicontrol-backend-skeleton (in `review`, not yet `done` — implemented in the prior session; only its real-AxiDraw-hardware verification is outstanding, unrelated to this ticket's scope)

## Execution Report

**Date:** 2026-08-05

Added a Dagger module (Go SDK) at `ci/` — a separate nested Go module (its own `go.mod`) so it doesn't entangle the app's dependency graph — with five functions: `Lint`, `Test`, `Build` (thin wrapper over `source.DockerBuild()`, deliberately agnostic to the Dockerfile's contents so it doesn't care that the final stage is Python-capable rather than `distroless/static`, per ADR-0014), `CI` (sequences the three, returning an error before `Build` runs if either `Lint` or `Test` fails), and `Publish` (runs `CI`, then pushes to GHCR only on success).

**Verified locally, this session:**
- `dagger call -m ./ci lint --source=.` and `test --source=.` — both pass against the real repo.
- `dagger call -m ./ci ci --source=.` — full lint→test→build chain; chained into the resulting container (`with-exec --args=axicli,--version`) to confirm the built image is the real one with `axicli` inside.
- Deliberately broke a lint rule (unused variable) in a scratch copy of the repo: `dagger call ci` failed at the lint step with exit code 1, never reaching build.
- Deliberately broke a test (in the same scratch copy): lint passed (cached), `go test` failed, pipeline stopped before build.
- `actionlint .github/workflows/ci.yml` — zero issues.
- `dagger run go test ./...` / `dagger run go vet ./...` inside `ci/` (the module's generated client needs a live Dagger session, so these can't run under plain `go test`) — both clean, including a new `ci/main_test.go` table-testing `imageTag`'s ref → tag derivation.

**One design correction made after code review:** the first workflow draft computed the image tag (`latest` vs. the git tag) and lowercased the owner in `.github/workflows/ci.yml` shell steps. Both the Standards and Spec review sub-agents independently flagged this as being in tension with ADR-0013's "runs identically in GitHub Actions, another CI provider, or a developer's machine" intent and the acceptance criterion that the workflow contain "only the trigger and the call into the Dagger pipeline" — real conditional logic living only in YAML breaks that portability. Moved it into `Publish`/`imageTag` in `ci/main.go` instead; the workflow now passes only raw GitHub context values (`github.ref`, `github.repository_owner`, `github.actor`) with no computation of its own.

**Not verified — needs a real push:** an actual `git push` to `main` or a version tag triggering the GitHub Actions workflow and landing an image in GHCR. That requires the repo's `GITHUB_TOKEN`/GHCR permissions in a live GitHub Actions run, which this session can't exercise — the pipeline logic itself is exercised end-to-end locally via `dagger call`, which is exactly what ADR-0013 and this ticket's local-parity acceptance criterion are asking to be true.
