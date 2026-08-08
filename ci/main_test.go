package main

import (
	"testing"

	"dagger/axicontrol/internal/dagger"
)

func TestBuildPlatforms(t *testing.T) {
	want := []dagger.Platform{"linux/amd64", "linux/arm64"}
	if len(buildPlatforms) != len(want) {
		t.Fatalf("buildPlatforms = %v, want %v", buildPlatforms, want)
	}
	for i, p := range want {
		if buildPlatforms[i] != p {
			t.Errorf("buildPlatforms[%d] = %q, want %q", i, buildPlatforms[i], p)
		}
	}
}

func TestCIPlatformIsPublished(t *testing.T) {
	for _, p := range buildPlatforms {
		if p == ciPlatform {
			return
		}
	}
	t.Errorf("ciPlatform %q not present in buildPlatforms %v — ci.yml would verify a platform Publish doesn't ship", ciPlatform, buildPlatforms)
}

func TestImageTag(t *testing.T) {
	tests := []struct {
		name   string
		gitRef string
		want   string
	}{
		{name: "version tag", gitRef: "refs/tags/v0.1.0", want: "v0.1.0"},
		{name: "push to main", gitRef: "refs/heads/main", want: "latest"},
		{name: "unrecognized ref falls back to latest", gitRef: "refs/heads/some-branch", want: "latest"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := imageTag(tt.gitRef); got != tt.want {
				t.Errorf("imageTag(%q) = %q, want %q", tt.gitRef, got, tt.want)
			}
		})
	}
}
