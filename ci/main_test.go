package main

import "testing"

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
