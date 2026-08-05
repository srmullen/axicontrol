// A Dagger module implementing axicontrol's CI pipeline: lint, test, build,
// and publish the container image to GHCR. Written in Go so the same logic
// runs identically via the Dagger CLI, in GitHub Actions, or on a developer's
// machine — see ADR-0013.
package main

import (
	"context"
	"fmt"
	"strings"

	"dagger/axicontrol/internal/dagger"
)

const (
	goImage         = "golang:1.26-alpine"
	golangciLintTag = "v2.12.2-alpine"
)

type Axicontrol struct{}

// Lint runs golangci-lint against the source.
func (m *Axicontrol) Lint(ctx context.Context, source *dagger.Directory) (string, error) {
	return m.goContainer(source, "golangci/golangci-lint:"+golangciLintTag).
		WithExec([]string{"golangci-lint", "run", "./..."}).
		Stdout(ctx)
}

// Test runs the Go test suite against the source.
func (m *Axicontrol) Test(ctx context.Context, source *dagger.Directory) (string, error) {
	return m.goContainer(source, goImage).
		WithExec([]string{"go", "test", "./..."}).
		Stdout(ctx)
}

// Build produces the container image defined by the repo's multi-stage
// Dockerfile (see ADR-0014 for why the final stage is Python-capable rather
// than distroless/static).
func (m *Axicontrol) Build(source *dagger.Directory) *dagger.Container {
	return source.DockerBuild()
}

// CI runs Lint, then Test, then Build — the same steps GitHub Actions runs
// before publishing, runnable locally with no registry credentials involved:
//
//	dagger call -m ./ci ci --source=. export --path=/tmp/axicontrol.tar
//
// A lint or test failure returns an error before Build ever runs.
func (m *Axicontrol) CI(ctx context.Context, source *dagger.Directory) (*dagger.Container, error) {
	if _, err := m.Lint(ctx, source); err != nil {
		return nil, fmt.Errorf("lint: %w", err)
	}
	if _, err := m.Test(ctx, source); err != nil {
		return nil, fmt.Errorf("test: %w", err)
	}
	return m.Build(source), nil
}

// Publish runs CI (lint, test, build) and, only if all three succeed, pushes
// the resulting image to GHCR. owner is the GHCR namespace (e.g. the GitHub
// org/user) used for the image path; registryUsername is the identity to
// authenticate to GHCR as (e.g. the actor running the workflow), which is
// not always the same as owner. gitRef is the triggering ref as GitHub
// Actions reports it (e.g. "refs/heads/main" or "refs/tags/v0.1.0") — tag
// selection lives here rather than in workflow YAML so that running this
// function outside GitHub Actions produces the same outcome (ADR-0013).
func (m *Axicontrol) Publish(
	ctx context.Context,
	source *dagger.Directory,
	owner string,
	registryUsername string,
	registryPassword *dagger.Secret,
	gitRef string,
) (string, error) {
	image, err := m.CI(ctx, source)
	if err != nil {
		return "", err
	}

	image = image.WithRegistryAuth("ghcr.io", registryUsername, registryPassword)

	ref := fmt.Sprintf("ghcr.io/%s/axicontrol:%s", strings.ToLower(owner), imageTag(gitRef))
	addr, err := image.Publish(ctx, ref)
	if err != nil {
		return "", fmt.Errorf("publish %s: %w", ref, err)
	}
	return addr, nil
}

// imageTag derives the GHCR tag from a git ref: a version-tag push (e.g.
// "refs/tags/v0.1.0") publishes under that tag name, so the homelab repo can
// pin to a specific version; anything else (i.e. a push to main, the only
// other trigger configured) publishes "latest".
func imageTag(gitRef string) string {
	if tag, ok := strings.CutPrefix(gitRef, "refs/tags/"); ok {
		return tag
	}
	return "latest"
}

// goContainer mounts source into a container built from baseImage, with the
// module cache warmed from go.mod/go.sum before the rest of the source is
// added, so dependency downloads are cached across lint/test runs.
func (m *Axicontrol) goContainer(source *dagger.Directory, baseImage string) *dagger.Container {
	return dag.Container().
		From(baseImage).
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("axicontrol-go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("axicontrol-go-build")).
		WithDirectory("/src", source, dagger.ContainerWithDirectoryOpts{
			Exclude: []string{".git", "ci", "data"},
		}).
		WithWorkdir("/src")
}
