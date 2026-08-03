---
name: axicontrol-ci-pipeline
description: 'Dagger-driven CI: lint, test, build, and publish the container image to GHCR'
lane: backlog
tags: ready-for-agent
created-at: "2026-08-03"
created-by: seanmullen
depends-on: axicontrol-backend-skeleton
---

## Parent

Derived from a `/grill-with-docs` tech-stack session. See [ADR-0013](../docs/adr/0013-build-ci-and-publish.md) (build, CI, and publish).

## What to build

Automated CI that lints, tests, builds, and publishes the container image from axicontrol-backend-skeleton — with the pipeline logic itself written portably (Dagger), not locked into GitHub-Actions-specific YAML.

- A Dagger module (Go SDK) defining the pipeline: `golangci-lint` → `go test` → `docker build` (the multi-stage Dockerfile → `distroless/static` image from axicontrol-backend-skeleton) → publish.
- A thin GitHub Actions workflow that triggers the Dagger pipeline on push to `main` and on version tags — the workflow itself contains no build/test/lint logic, only "run the Dagger pipeline."
- Images published to GHCR (`ghcr.io/<owner>/axicontrol`): `latest` on `main`, a matching semver tag on version tags — so the homelab repo can pin to a specific tag.
- A lint or test failure stops the pipeline before it attempts to build or publish.

## Acceptance criteria

- [ ] Pushing to `main` runs lint, then test, then build; on success an image is published to `ghcr.io/<owner>/axicontrol:latest`
- [ ] Pushing a version tag (e.g. `v0.1.0`) publishes an image tagged to match
- [ ] The same Dagger pipeline runs locally (e.g. `dagger call ...`) and produces the same lint/test/build outcome without GitHub Actions involved
- [ ] A failing `golangci-lint` or `go test` run stops the pipeline before the build/publish steps run
- [ ] The GitHub Actions workflow file contains no lint/test/build logic itself — only the trigger and the call into the Dagger pipeline

## Blocked by

- axicontrol-backend-skeleton
