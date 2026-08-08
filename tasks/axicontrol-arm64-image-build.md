---
name: axicontrol-arm64-image-build
description: Build multi-arch (amd64+arm64) container images so the app can deploy to the Raspberry Pi node without an exec format error
lane: review
tags: ready-for-agent
created-at: "2026-08-08"
created-by: seanmullen
assigned-to: claude
---

## Parent

Derived from a `/grill-with-docs` session. See [ADR-0016](../docs/adr/0016-multi-arch-image-build.md) (multi-arch image build, QEMU-emulated, arm64 verified only at release), which supersedes part of [ADR-0013](../docs/adr/0013-build-ci-and-publish.md).

## Problem Statement

The AxiDraw hardware is physically wired to one specific node in the homelab k8s cluster ([ADR-0001](../docs/adr/0001-node-pinned-device-access.md)), and that node is a 64-bit Raspberry Pi (`linux/arm64`). The image built by `ci/main.go`'s `Build`/`Publish` functions is only ever built for `linux/amd64` (Dagger's `Directory.DockerBuild()` defaults to the Dagger engine host's platform, which is `amd64` on the GitHub Actions runner). Deploying that image to the Pi node fails immediately with `exec /axicontrold: exec format error` — the container runtime can't execute an amd64 ELF binary on an arm64 kernel.

## Solution

Publish a multi-arch image (`linux/amd64` + `linux/arm64` under one tag) so the same tag the homelab repo references works on the Pi node. Build both platform variants entirely under QEMU emulation rather than cross-compiling — this requires no Dockerfile changes, since `go build` (no explicit `GOARCH`) already compiles correctly for whichever arch the emulated container reports natively. The push-to-`main` `ci.yml` workflow keeps verifying only `amd64` (fast, no emulation); the arm64 build only happens in `release.yml`, at the deliberate release step, alongside the existing `CI` gate re-run.

## User Stories

1. As a repo maintainer, I want the published image to run on the Pi node without `exec format error`, so that a GitHub Release actually deploys.
2. As a repo maintainer, I want the same image tag to also run on an amd64 dev machine, so that I can `docker run` it locally for testing without cross-arch surprises.
3. As a developer pushing to `main`, I want `ci.yml` to stay fast and emulation-free, so that routine pushes aren't slowed down by an arm64 build that release time will verify anyway.
4. As a release manager, I want `release.yml` to build and verify both platforms before publishing, so that a broken arm64 build can't reach GHCR under a real version tag.
5. As a repo maintainer, I want the Dagger pipeline (not GitHub Actions YAML) to own the platform list, so that `dagger call -m ./ci publish ...` produces the same multi-arch result locally as it does in CI, per ADR-0013's portability goal.

## Implementation Decisions

- **Target platforms.** `linux/amd64` and `linux/arm64` (`arm64` chosen over `armv7` — the Pi node runs a 64-bit OS).
- **QEMU-emulated whole-build, not cross-compile.** `Directory.DockerBuild(Platform: "linux/arm64")` emulates every stage (Go compile, `apt-get`, `pip install`) rather than using `--platform=$BUILDPLATFORM` + `TARGETARCH` to cross-compile just the Go stage natively. Simpler (zero Dockerfile changes) at the cost of slower arm64 builds — acceptable since arm64 only builds at release time, not on every push.
- **`release.yml` needs a QEMU/binfmt registration step** (e.g. `docker/setup-qemu-action`) added before the Dagger action runs — GitHub's standard `ubuntu-latest` runner has no arm64 binfmt handler registered by default, and Dagger's buildkit-based engine depends on the host having one to execute foreign-arch binaries under emulation.
- **`ci/main.go` — `Build` takes an explicit `Platform` parameter** instead of relying on Dagger's implicit host-default platform. This matters for ADR-0013's "runs identically on a developer's machine" claim: on an Apple Silicon dev machine, the implicit default would be `arm64`, not `amd64`, silently changing what `CI`'s build step verifies.
- **`ci/main.go` — `CI` (called by `ci.yml`) pins `Build` to `linux/amd64` only.** No behavior change to the fast push-to-main path beyond making the platform explicit.
- **`ci/main.go` — `Publish` builds both `linux/amd64` and `linux/arm64`,** merging them into one manifest via `Container.Publish`'s `PlatformVariants` option, published under the existing tag scheme (`imageTag()` unchanged).
- **No Dockerfile changes.** `GOOS=linux` stays hardcoded (no `GOARCH` override needed); `golang:1.26-alpine` and `python:3.12-slim` are both official multi-arch images that already publish `linux/arm64` variants.
- **Documentation:** add `docs/adr/0016-multi-arch-image-build.md` recording this decision (QEMU-emulated multi-platform Dagger build, `amd64`-only CI vs `amd64`+`arm64` release) and its rejected alternatives (arm64-only image, cross-compile via `TARGETARCH`, native arm64 GitHub runners); annotate ADR-0013 with a "superseded in part" note, following the existing 0014/0015 pattern.

## Testing Decisions

- `ci/main_test.go` — extend/add coverage for the platform list `Publish` builds against (e.g. a table test or assertion that both `linux/amd64` and `linux/arm64` are included), consistent with existing `imageTag()` table-test style.
- `dagger call -m ./ci ci --source=.` should still succeed and remain amd64-only/fast — verify no arm64 emulation is triggered by this path.
- `dagger call -m ./ci publish --source=. ... ` (with a scratch/dry-run registry or `--help` signature check, as in `axicontrol-split-ci-release-workflow`) — confirm both platform variants build and the argument signature is otherwise unchanged.
- `actionlint .github/workflows/release.yml` after adding the QEMU setup step.
- **Not verifiable in an agent session:** actually running the published multi-arch image on a real Raspberry Pi node, or a live GitHub Actions run publishing to GHCR — both require hardware/secrets outside this repo's scope, same limitation flagged in prior CI tickets (`axicontrol-ci-pipeline`, `axicontrol-split-ci-release-workflow`). Flag explicitly as not-live-verified rather than claimed working end-to-end.

## Out of Scope

- Cross-compiling via `--platform=$BUILDPLATFORM` + `TARGETARCH` (rejected in favor of whole-build QEMU emulation — see Implementation Decisions).
- Native arm64 GitHub-hosted runners / matrix workflow restructuring (rejected — adds workflow complexity inconsistent with ADR-0013's "thin trigger" principle, for a low-frequency release pipeline).
- Building arm64 on every push to `main` in `ci.yml` (rejected — arm64 verification is deferred to release time, where `Publish` already re-runs the full `CI` gate per ADR-0015).
- Any change to the homelab repo's k8s manifests, node labeling, or `nodeAffinity` config (out of scope for this repo per ADR-0013).
- `armv7`/32-bit Pi support.

## Further Notes

- Confirmed via web search that AxiDraw's Python tooling (`axidrawinternal`, `pyserial`, `lxml`, `numpy`) has prior art running on a Raspberry Pi and ships arm64 wheels, so this isn't expected to be blocked at the `pip install` step — see [Evil Mad Scientist forum: "Axidraw & Raspberry Pi 4"](https://www.evilmadscientist.com/forums/topic/axidraw-raspberry-pi-4/).
- The homelab repo (separate repo, out of scope here) should be checked once this ships, to confirm it deploys the newly multi-arch tag rather than a stale amd64-only digest it may have cached/pinned.

## Execution Report

**Date:** 2026-08-08

- `ci/main.go`: added `buildPlatforms` (`[]dagger.Platform{"linux/amd64", "linux/arm64"}`) and `ciPlatform` (`"linux/amd64"`). `Build` now takes an explicit `platform dagger.Platform` parameter and passes it to `Directory.DockerBuild`'s `Platform` option (no Dockerfile changes — `go build` has no `GOARCH` override, so it compiles for whatever arch the emulated container reports natively). `CI` calls `Build(source, ciPlatform)`. `Publish` runs `CI` as a gate (discarding its container, keeping only the error), then builds every platform in `buildPlatforms` and publishes them as one multi-arch manifest via a fresh `dag.Container().WithRegistryAuth(...).Publish(ctx, ref, dagger.ContainerPublishOpts{PlatformVariants: variants})`, rather than publishing from one of the platform-specific containers directly.
- `ci/main_test.go`: added `TestBuildPlatforms` (asserts the exact platform list) and `TestCIPlatformIsPublished` (asserts `ciPlatform` is a member of `buildPlatforms`, so `ci.yml` can't silently verify a platform `Publish` doesn't ship).
- `.github/workflows/release.yml`: added a `docker/setup-qemu-action@v3` step (`platforms: arm64`) before the Dagger action, registering the binfmt handler `ubuntu-latest` doesn't have by default.
- `docs/adr/0016-multi-arch-image-build.md` (new): records the decision and rejected alternatives (arm64-only image, cross-compile via `TARGETARCH`/`BUILDPLATFORM`, native arm64 GitHub runners, building arm64 in `ci.yml` on every push).
- `docs/adr/0013-build-ci-and-publish.md`: annotated with a "superseded in part by ADR-0016" paragraph, following the existing 0014/0015 pattern.
- Ran `dagger develop` in `ci/` to regenerate the gitignored `dagger.gen.go`/`internal/dagger` codegen for `Build`'s new signature (these files aren't tracked in git — see `ci/.gitignore`).

**Verified locally, this session:**
- `go build ./... && go vet ./... && go test ./...` (repo root) — main app untouched, all packages pass.
- `dagger run go vet ./...` and `dagger run go test ./... -v` (inside `ci/`) — new `TestBuildPlatforms`/`TestCIPlatformIsPublished` pass alongside the existing `imageTag` table test.
- `actionlint .github/workflows/ci.yml .github/workflows/release.yml` — zero issues.
- `dagger call -m ./ci ci --source=.` — full lint→test→build(amd64) gate succeeds unchanged.
- `dagger call -m ./ci build --source=. --platform=linux/arm64 with-exec --args=axicli,--version stdout` — confirms `axicli` runs inside the arm64 image (this session's dev machine is Apple Silicon, so this build is native; the `linux/amd64` build was the one exercising QEMU emulation locally).
- Directly inspected the compiled `/axicontrold` binary's ELF header (`od -An -tx1 -N20`) in both platform builds: `e_machine` bytes are `b7 00` (`EM_AARCH64` = 183) for the `linux/arm64` build and `3e 00` (`EM_X86_64` = 62) for the `linux/amd64` build — confirms the Go compiler is genuinely producing arch-correct binaries for each target under emulation, not silently reusing the host's native arch.
- `dagger call -m ./ci publish --help` — confirmed the `publish` function's exposed argument signature (`--source`, `--owner`, `--registry-username`, `--registry-password`, `--git-ref`) is unchanged and matches `release.yml`'s call exactly.

**Not verified — needs a live GitHub Actions run:** an actual GitHub Release triggering `release.yml`, confirming `docker/setup-qemu-action` correctly enables the emulated `linux/arm64` build on `ubuntu-latest` in that environment (this session could only verify emulation works on a local Apple Silicon Docker setup, which already ships binfmt/QEMU support out of the box — GitHub's bare Linux runner is the untested case this ticket specifically added the `setup-qemu-action` step for), and running the resulting published image on the actual Raspberry Pi node. Same limitation as prior CI tickets — requires repo secrets and real hardware outside this session's reach.
