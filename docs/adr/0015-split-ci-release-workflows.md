# Split CI into test and release workflows; publish only on GitHub Release, drop `latest` tag

[ADR-0013](./0013-build-ci-and-publish.md) described GitHub Actions as a thin trigger "on push/tag" invoking the Dagger `Publish` function, which pushed an image on every push to `main` (tagged `latest`) and on any `v*` tag push. This tied image publication to the shape of `main`'s history rather than to a deliberate release action, and the `latest` tag gave homelab deployments a floating target with no version pinned in the tag itself.

GitHub Actions is now split into two workflows. `ci.yml` runs Dagger's `Lint` → `Test` → `Build` (the `CI` function) on every push to `main`, giving fast feedback without ever touching the registry. `release.yml` runs Dagger's `Publish` function — which still re-runs `CI` internally as a gate — only on the `release: published` event, including pre-releases. Publishing to GHCR now only happens as the outcome of a human deliberately creating a GitHub Release with a specific tag; pushing a `v*` tag alone triggers nothing, and `main` no longer publishes a `latest` image at all.

## Considered Options

- Keep the single combined workflow, trigger release publish on tag push (`v*`) instead of the `release` event — rejected: a tag push is a lower-friction, less deliberate action than authoring a GitHub Release, and doesn't distinguish "I'm testing tagging" from "I'm cutting a release."
- Keep publishing `latest` on every push to `main`, alongside versioned release publishes — rejected: a floating tag with no gate visible in the tag name itself; any consumer pinning to it inherits whatever landed on `main` most recently, defeating the point of tying publishes to deliberate releases.
- Skip re-running lint/test/build in the release workflow, since `ci.yml` already gated the commit on push to `main` — rejected: a release can be cut from a commit reached by paths other than "just landed on main" (e.g. re-releasing an older tag), and the CI time cost is cheap insurance against publishing a broken image under a permanent version tag.
