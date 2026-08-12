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

func TestPrintPageModeSelectorIncludesSingleLayerOptionWhenLayersExist(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	body := printPage(t, s, fileID).Body.String()

	require.Contains(t, body, `value="single_layer"`)
	require.Contains(t, body, "Single Layer")
}

func TestPrintPageJobFormIncludesLivePreviewFieldsInSubmission(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	body := printPage(t, s, fileID).Body.String()

	require.Contains(t, body, `hx-include="#print-preview"`, "the job form must pull mode/layer_number in from the live-preview area, which sits outside the <form>")
}

func TestUploadPreviewFragmentDefaultsToWholeFileImage(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/preview", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "/uploads/"+itoa(fileID)+"/content")
}

func TestUploadPreviewFragmentSwitchesToChosenLayerImage(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/preview?mode=single_layer&layer_number=2", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	require.Contains(t, body, "/uploads/"+itoa(fileID)+"/layers/2/content")
	require.NotContains(t, body, `src="/uploads/`+itoa(fileID)+`/content"`)
}

func TestUploadPreviewFragmentMalformedLayerNumberReturnsBadRequest(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/preview?mode=single_layer&layer_number=not-a-number", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
	require.Contains(t, rr.Body.String(), "layer number must be an integer")
}

func TestUploadPreviewFragmentSingleLayerModeShowsLayerDropdown(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s) // layers 1 (black), 2 (red)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/preview?mode=single_layer", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	require.Contains(t, body, `name="layer_number"`)
	require.Contains(t, body, "1 — black")
	require.Contains(t, body, "2 — red")
	require.Contains(t, body, "/uploads/"+itoa(fileID)+"/content", "no layer chosen yet, so the whole-file image is still shown")
}

func TestUploadPreviewFragmentIgnoresALayerNumberNotBelongingToTheUpload(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s) // layers 1 and 2 only

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/preview?mode=single_layer&layer_number=99", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	require.NotContains(t, body, "/layers/99/content", "a layer number the upload doesn't have must not be built into an <img src>")
	require.Contains(t, body, `src="/uploads/`+itoa(fileID)+`/content"`, "falls back to the whole-file image instead")
}

func TestUploadPreviewFragmentWholeModeHidesLayerDropdown(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/preview?mode=whole", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.NotContains(t, rr.Body.String(), `name="layer_number"`)
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

// startBusyJob submits a Job against busyFileID whose axicli invocation
// blocks on release, leaving it "printing" (and the device claimed) until
// the caller closes release, for tests that need another Upload's print
// page to observe the device as busy.
func startBusyJob(t *testing.T, s *Server, busyFileID, presetID int64) (release chan struct{}) {
	t.Helper()
	started := make(chan struct{})
	release = make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, busyFileID, url.Values{"preset_id": {strconv.FormatInt(presetID, 10)}})
	require.Equal(t, http.StatusOK, rr.Code)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	return release
}

func busyStatus(t *testing.T, s *Server, uploadID int64) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(uploadID)+"/busy-status", nil)
	s.ServeHTTP(rr, req)
	return rr
}

func TestPrintPageShowsBusyBannerForAnotherUploadsRunningJob(t *testing.T) {
	s := newTestServer(t)
	busyFileID, presetID := seedFileAndPreset(t, s) // "drawing.svg"
	release := startBusyJob(t, s, busyFileID, presetID)
	defer close(release)

	rr := doMultipartUpload(t, s, "other.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	otherID := firstUploadID(t, rr.Body.String())

	body := printPage(t, s, otherID).Body.String()
	require.Contains(t, body, "AxiDraw is busy")
	require.Contains(t, body, "drawing.svg", "the busy banner must name which Upload's Job the device is running")
	require.Contains(t, body, "Pass 1/1")
}

func TestPrintPageOwnInProgressJobDoesNotAlsoShowBusyBanner(t *testing.T) {
	s := newTestServer(t)
	busyFileID, presetID := seedFileAndPreset(t, s)
	release := startBusyJob(t, s, busyFileID, presetID)
	defer close(release)

	body := printPage(t, s, busyFileID).Body.String()
	require.Contains(t, body, "print-job-", "the owning page still shows its own in-progress Job inline")
	require.NotContains(t, body, "AxiDraw is busy", "a print page must not tell its own Upload's owner the device is busy with itself")
}

func TestPrintPageNoBusyBannerWhenDeviceIsFree(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedFileAndPreset(t, s)

	body := printPage(t, s, fileID).Body.String()
	require.NotContains(t, body, "AxiDraw is busy")
	require.NotContains(t, body, "busy-status", "a print page with nothing to watch must not poll forever")
}

func TestPrintPageBusyBannerPollsForLiveUpdatesWhileTheDeviceIsBusy(t *testing.T) {
	s := newTestServer(t)
	busyFileID, presetID := seedFileAndPreset(t, s)
	release := startBusyJob(t, s, busyFileID, presetID)
	defer close(release)

	rr := doMultipartUpload(t, s, "other.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	otherID := firstUploadID(t, rr.Body.String())

	body := printPage(t, s, otherID).Body.String()
	require.Contains(t, body, `id="device-busy-banner"`)
	require.Contains(t, body, `hx-get="/uploads/`+itoa(otherID)+`/busy-status"`)
	require.Contains(t, body, `hx-trigger="every 2s"`)
}

func TestBusyStatusFragmentShowsBusyJobForADifferentUpload(t *testing.T) {
	s := newTestServer(t)
	busyFileID, presetID := seedFileAndPreset(t, s)
	release := startBusyJob(t, s, busyFileID, presetID)
	defer close(release)

	rr := doMultipartUpload(t, s, "other.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	otherID := firstUploadID(t, rr.Body.String())

	body := busyStatus(t, s, otherID).Body.String()
	require.Contains(t, body, "AxiDraw is busy")
	require.Contains(t, body, "drawing.svg")
}

func TestBusyStatusFragmentClearsOnceTheRunningJobFinishes(t *testing.T) {
	s := newTestServer(t)
	busyFileID, presetID := seedFileAndPreset(t, s)
	release := startBusyJob(t, s, busyFileID, presetID)

	rr := doMultipartUpload(t, s, "other.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	otherID := firstUploadID(t, rr.Body.String())

	require.Contains(t, busyStatus(t, s, otherID).Body.String(), "AxiDraw is busy")

	close(release)
	require.Eventually(t, func() bool {
		return !strings.Contains(busyStatus(t, s, otherID).Body.String(), "AxiDraw is busy")
	}, 2*time.Second, 10*time.Millisecond, "the busy banner must clear once the running Job finishes, without a reload")
}

func TestBusyStatusFragmentExcludesOwnUpload(t *testing.T) {
	s := newTestServer(t)
	busyFileID, presetID := seedFileAndPreset(t, s)
	release := startBusyJob(t, s, busyFileID, presetID)
	defer close(release)

	require.NotContains(t, busyStatus(t, s, busyFileID).Body.String(), "AxiDraw is busy")
}

func TestBusyStatusFragmentMissingUploadReturnsNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := busyStatus(t, s, 999)
	require.Equal(t, http.StatusNotFound, rr.Code)
}
