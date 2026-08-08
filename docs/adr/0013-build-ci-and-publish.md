# Build, CI, and publish: Dagger pipeline, GitHub Actions trigger, GHCR

This repo builds and publishes a container image; it does not own the k8s deployment manifests (Deployment, PV/PVC, node affinity, hostPath device mount from [ADR-0001](./0001-node-pinned-device-access.md)) — those live in a separate homelab repo. This repo's only obligation to that repo is producing an image that's trivially deployable.

The image is a multi-stage Dockerfile with `gcr.io/distroless/static` as the final stage — viable specifically because the backend is a pure-Go static binary with no cgo dependency ([ADR-0011](./0011-backend-stack.md)'s pure-Go SQLite driver is what makes this possible). Distroless has no shell and the smallest practical attack surface; `scratch` would be marginally smaller but loses CA certificates needed for the outbound webhook POSTs ([ADR-0006](./0006-webhook-and-sse-notifications.md)) for no real benefit at this size.

**Superseded in part by [ADR-0014](./0014-python-base-image-for-axicli.md):** the `distroless/static` final stage described above didn't account for `axicli` needing a Python runtime to run as a subprocess ([ADR-0001](./0001-node-pinned-device-access.md)). The final stage is `python:3.12-slim` instead; everything else here (Dagger pipeline, GitHub Actions trigger, GHCR publish target) stands.

CI logic — build, `golangci-lint`, test, publish — is written as a Dagger module rather than as native GitHub Actions YAML steps, with GitHub Actions kept as a thin trigger (on push/tag) that invokes the Dagger pipeline. This is a deliberate portability choice: the pipeline is expressed in code that runs identically in GitHub Actions, another CI provider, or a developer's local machine, rather than being written directly in a CI-provider-specific format that only runs there. The built image is published to GHCR, since it's free with the GitHub repo and needs no separate registry account — the homelab repo just references an image tag.

**Superseded in part by [ADR-0015](./0015-split-ci-release-workflows.md):** the "thin trigger (on push/tag)" described above is now two triggers — a push-to-`main` trigger that runs lint/test/build with no publish, and a `release: published` trigger that runs the full `Publish` pipeline. Publishing on every push to `main` (tagged `latest`) is gone; everything else here (Dagger pipeline as the portable CI logic, GHCR as the publish target) stands.

**Superseded in part by [ADR-0016](./0016-multi-arch-image-build.md):** the image is no longer single-platform. `Publish` now builds and merges `linux/amd64` and `linux/arm64` into one multi-arch manifest (the k8s node the AxiDraw is wired to, per [ADR-0001](./0001-node-pinned-device-access.md), is a Raspberry Pi); `ci.yml` still verifies only `linux/amd64` on push to `main`. Everything else here stands.

## Considered Options

- Docker Hub instead of GHCR — rejected: no existing standardization on Docker Hub for other homelab images; GHCR is free and closer to the source repo.
- CI logic written directly as GitHub Actions YAML — rejected: ties the build/test/lint/publish logic to GitHub Actions specifically, with no path to running it elsewhere (another CI provider, or locally) without rewriting it.
- `scratch` base image instead of `distroless/static` — rejected: saves negligible size, loses CA certs needed for webhook delivery.
- k8s manifests (Helm/Kustomize/plain YAML) — explicitly out of scope for this repo; they live in the homelab repo, which decides its own manifest tooling.
