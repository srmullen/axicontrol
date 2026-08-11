package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDryRunReturnsOutputWithoutClaimingDevice(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("This plot would take approximately 12 seconds."), nil
	}

	rr := doForm(t, s, http.MethodPost, "/jobs/dry-run", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "12 seconds")

	require.Contains(t, gotArgs, "--preview")
	require.Contains(t, gotArgs, "--report_time")
	require.NotContains(t, gotArgs, "-o", "a dry run must never write an output/checkpoint file")

	// A dry run must never claim the device — a real job submitted right
	// after it must succeed, not be rejected as "already printing".
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}
	rr = submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotContains(t, rr.Body.String(), "already printing")
}

func TestDryRunUsesSameConfigResolutionAsRealPrint(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("ok"), nil
	}

	rr := doForm(t, s, http.MethodPost, "/jobs/dry-run", url.Values{
		"file_id":       {strconv.FormatInt(fileID, 10)},
		"preset_id":     {strconv.FormatInt(presetID, 10)},
		"speed_pendown": {"5"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	require.Contains(t, gotArgs, "5", "the pass-level override must be applied")
	require.NotContains(t, gotArgs, "25", "the preset's un-overridden default must not leak through instead of the override")
	require.Contains(t, gotArgs, "--model")
	require.Contains(t, gotArgs, "--penlift")
}

func TestDryRunRejectsUnknownFile(t *testing.T) {
	s := newTestServer(t)
	_, presetID := seedFileAndPreset(t, s)

	rr := doForm(t, s, http.MethodPost, "/jobs/dry-run", url.Values{
		"file_id":   {"999"},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "file not found")
}

func TestDryRunRejectsUnknownPreset(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedFileAndPreset(t, s)

	rr := doForm(t, s, http.MethodPost, "/jobs/dry-run", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {"999"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "preset not found")
}

func TestDryRunSurfacesAxicliFailure(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("device error"), errors.New("exit status 1")
	}

	rr := doForm(t, s, http.MethodPost, "/jobs/dry-run", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	// htmx only swaps a fragment into the DOM on a 2xx response by default,
	// so the inline error must ship as 200 to actually render.
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "device error")
}

func TestDryRunSurfacesLaunchFailureWithNoOutput(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		// A launch-level failure (e.g. the axicli binary itself missing)
		// produces no combined output at all — only a Go error.
		return nil, errors.New(`exec: "axicli": executable file not found in $PATH`)
	}

	rr := doForm(t, s, http.MethodPost, "/jobs/dry-run", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "executable file not found",
		"the error must still surface even when axicli produced no output at all")
}

func TestDryRunRunsWhileAnotherJobIsPrinting(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	started := make(chan struct{})
	release := make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	<-started
	defer close(release)

	// The real print's axicli invocation is currently blocked; a dry-run
	// must still be able to run its own, independent invocation — --preview
	// never talks to the device, so it isn't subject to the print's
	// exclusive device claim.
	dryRunCalled := make(chan struct{})
	realRunAxicli := s.runAxicli
	s.runAxicli = func(ctx context.Context, args ...string) ([]byte, error) {
		for _, a := range args {
			if a == "--preview" {
				close(dryRunCalled)
				return []byte("preview ok"), nil
			}
		}
		return realRunAxicli(ctx, args...)
	}

	rr = doForm(t, s, http.MethodPost, "/jobs/dry-run", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "preview ok")

	select {
	case <-dryRunCalled:
	default:
		t.Fatal("dry run did not invoke axicli")
	}
}
