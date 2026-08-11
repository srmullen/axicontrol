package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPrintPageShowsWholeDocumentPreview(t *testing.T) {
	s := newTestServer(t)

	rr := doMultipartUpload(t, s, "drawing.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	id := firstUploadID(t, rr.Body.String())

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(id)+"/print", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	body := rr.Body.String()
	require.Contains(t, body, "drawing.svg")
	require.Contains(t, body, "<img")
	require.Contains(t, body, "/uploads/"+itoa(id)+"/content")
	require.NotContains(t, body, "<object")
	require.NotContains(t, body, "<iframe")
}

func TestPrintPageMissingUploadReturnsNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/999/print", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPrintPageInvalidIDReturnsBadRequest(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/not-a-number/print", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func printPage(t *testing.T, s *Server, id int64) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(id)+"/print", nil)
	s.ServeHTTP(rr, req)
	return rr
}

func TestPrintPageModeSelectorAbsentWithoutDiscoveredLayers(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedFileAndPreset(t, s) // validSVG has no numbered layers

	body := printPage(t, s, fileID).Body.String()

	require.NotContains(t, body, `name="mode"`, "the mode selector must be absent, not merely disabled, with zero discovered layers")
	require.NotContains(t, body, "Layers (one Pass")
}

func TestPrintPageModeSelectorPresentWithDiscoveredLayers(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	body := printPage(t, s, fileID).Body.String()

	require.Contains(t, body, `name="mode"`)
	require.Contains(t, body, "Layers (one Pass")
}

func TestPrintPageHasSubmissionFormBoundToThisUpload(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedFileAndPreset(t, s)

	body := printPage(t, s, fileID).Body.String()

	require.Contains(t, body, `hx-post="/uploads/`+itoa(fileID)+`/jobs"`)
	require.NotContains(t, body, `name="file_id" required`, "no file picker: the file is bound by the page's own URL")
}

func TestPrintPageSubmitWholeModeShowsLiveStatusWithoutNavigating(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	argsCh := make(chan []string, 1)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("plot complete"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{"preset_id": {strconv.FormatInt(presetID, 10)}})
	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	require.Contains(t, body, "print-job-", "the submission response itself must show the new Job's live status inline")
	jobID := firstPrintJobID(t, body)

	select {
	case <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	require.Eventually(t, func() bool {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/jobs/"+strconv.FormatInt(jobID, 10)+"/print-status", nil)
		s.ServeHTTP(rr, req)
		return strings.Contains(rr.Body.String(), "complete")
	}, 2*time.Second, 10*time.Millisecond)
}

func TestPrintPageSubmitLayersModeStartsFirstLayer(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)

	argsCh := make(chan []string, 1)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("layer plotted"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	var gotArgs []string
	select {
	case gotArgs = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	require.Contains(t, gotArgs, "layers")
	require.Contains(t, gotArgs, "1", "the lowest-numbered layer must run first")
}

func TestPrintPageReloadWhileJobInProgressStillShowsLiveStatus(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	started := make(chan struct{})
	release := make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}
	defer close(release)

	rr := submitJob(t, s, fileID, url.Values{"preset_id": {strconv.FormatInt(presetID, 10)}})
	require.Equal(t, http.StatusOK, rr.Code)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	body := printPage(t, s, fileID).Body.String()
	require.Contains(t, body, "print-job-", "reloading the print page mid-Job must still show its live status")
	require.Contains(t, body, "printing")
}

func TestPrintPageAfterJobCompletesNoLongerShowsItAsInProgress(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{"preset_id": {strconv.FormatInt(presetID, 10)}})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)

	body := printPage(t, s, fileID).Body.String()
	require.NotContains(t, body, "print-job-", "a terminal Job is history, not something the print page shows inline")
}

func TestPrintPageSubmitRejectedWhileDeviceBusyWithAnotherUpload(t *testing.T) {
	s := newTestServer(t)
	busyFileID, presetID := seedFileAndPreset(t, s)

	started := make(chan struct{})
	release := make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}
	defer close(release)

	rr := submitJob(t, s, busyFileID, url.Values{"preset_id": {strconv.FormatInt(presetID, 10)}})
	require.Equal(t, http.StatusOK, rr.Code)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	rr2 := doMultipartUpload(t, s, "other.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr2.Code)
	otherID := firstUploadID(t, rr2.Body.String())

	rr = submitJob(t, s, otherID, url.Values{"preset_id": {strconv.FormatInt(presetID, 10)}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "already printing")
}
