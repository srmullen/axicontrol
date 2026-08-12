package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func basePreset() presetView {
	return presetView{
		ID:           7,
		Name:         "fine-detail",
		SpeedPendown: 25,
		SpeedPenup:   75,
		Accel:        75,
		PenPosDown:   40,
		PenPosUp:     60,
		PenRateLower: 50,
		PenRateRaise: 50,
		PenDelayDown: 1,
		PenDelayUp:   2,
		ConstSpeed:   false,
	}
}

func TestOverridesApplyLeavesBaseUnchangedWhenEmpty(t *testing.T) {
	base := basePreset()
	var ov overrides

	resolved := ov.apply(base)

	require.Equal(t, base, resolved)
}

func TestOverridesApplyOverridesOnlyProvidedFields(t *testing.T) {
	base := basePreset()
	speed := 10
	ov := overrides{SpeedPendown: &speed}

	resolved := ov.apply(base)

	require.Equal(t, 10, resolved.SpeedPendown)
	require.Equal(t, base.SpeedPenup, resolved.SpeedPenup, "unrelated fields must be untouched")
	require.Equal(t, base.Accel, resolved.Accel)
}

func TestOverridesApplyConstSpeedOverride(t *testing.T) {
	base := basePreset()
	require.False(t, base.ConstSpeed)
	on := true
	ov := overrides{ConstSpeed: &on}

	resolved := ov.apply(base)

	require.True(t, resolved.ConstSpeed)
}

func TestParseOverridesFormAllBlankYieldsNoOverrides(t *testing.T) {
	r := httpRequestWithForm(t, url.Values{})

	ov, err := parseOverridesForm(r)

	require.NoError(t, err)
	require.Equal(t, overrides{}, ov)
}

func TestParseOverridesFormParsesProvidedFields(t *testing.T) {
	r := httpRequestWithForm(t, url.Values{
		"speed_pendown": {"10"},
		"const_speed":   {"true"},
	})

	ov, err := parseOverridesForm(r)

	require.NoError(t, err)
	require.NotNil(t, ov.SpeedPendown)
	require.Equal(t, 10, *ov.SpeedPendown)
	require.NotNil(t, ov.ConstSpeed)
	require.True(t, *ov.ConstSpeed)
	require.Nil(t, ov.SpeedPenup)
}

func TestParseOverridesFormRejectsNonIntegerField(t *testing.T) {
	r := httpRequestWithForm(t, url.Values{"speed_pendown": {"fast"}})

	_, err := parseOverridesForm(r)

	require.Error(t, err)
	require.Contains(t, err.Error(), "speed_pendown")
}

func TestParseOverridesFormRejectsInvalidConstSpeed(t *testing.T) {
	r := httpRequestWithForm(t, url.Values{"const_speed": {"maybe"}})

	_, err := parseOverridesForm(r)

	require.Error(t, err)
}

func httpRequestWithForm(t *testing.T, form url.Values) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodPost, "/jobs", strings.NewReader(form.Encode()))
	require.NoError(t, err)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, r.ParseForm())
	return r
}

func TestBuildAxicliArgsNoDevicePath(t *testing.T) {
	cfg := basePreset()
	device := deviceConfigView{Model: 2, Penlift: 3}

	args := buildAxicliArgs(axicliTarget{filePath: "/data/files/a.svg"}, "/data/checkpoints/1.svg", cfg, device, "")

	require.Equal(t, []string{
		"/data/files/a.svg",
		"--mode", "plot",
		"--speed_pendown", "25",
		"--speed_penup", "75",
		"--accel", "75",
		"--pen_pos_down", "40",
		"--pen_pos_up", "60",
		"--pen_rate_lower", "50",
		"--pen_rate_raise", "50",
		"--pen_delay_down", "1",
		"--pen_delay_up", "2",
		"--model", "2",
		"--penlift", "3",
		"-o", "/data/checkpoints/1.svg",
	}, args)
}

func TestBuildAxicliArgsWithDevicePath(t *testing.T) {
	cfg := basePreset()
	device := deviceConfigView{Model: 1, Penlift: 1}

	args := buildAxicliArgs(axicliTarget{filePath: "/data/files/a.svg"}, "/data/checkpoints/1.svg", cfg, device, "/dev/axidraw")

	require.Contains(t, args, "--port")
	require.Equal(t, []string{"-o", "/data/checkpoints/1.svg"}, args[len(args)-2:])
}

func TestBuildAxicliArgsConstSpeedFlagOnlyWhenTrue(t *testing.T) {
	device := deviceConfigView{Model: 1, Penlift: 1}
	target := axicliTarget{filePath: "f.svg"}

	off := basePreset()
	off.ConstSpeed = false
	require.NotContains(t, buildAxicliArgs(target, "c.svg", off, device, ""), "--const_speed")

	on := basePreset()
	on.ConstSpeed = true
	require.Contains(t, buildAxicliArgs(target, "c.svg", on, device, ""), "--const_speed")
}

func TestBuildAxicliArgsAppliesOverriddenSpeed(t *testing.T) {
	device := deviceConfigView{Model: 1, Penlift: 1}
	base := basePreset()
	overridden := 99
	resolved := (overrides{SpeedPendown: &overridden}).apply(base)

	args := buildAxicliArgs(axicliTarget{filePath: "f.svg"}, "c.svg", resolved, device, "")

	require.Contains(t, args, "99")
	require.NotContains(t, args, "25")
}

func TestBuildAxicliArgsWholeModeUsesPlotMode(t *testing.T) {
	cfg := basePreset()
	device := deviceConfigView{Model: 1, Penlift: 1}

	args := buildAxicliArgs(axicliTarget{filePath: "f.svg"}, "c.svg", cfg, device, "")

	require.Equal(t, []string{"f.svg", "--mode", "plot"}, args[:3])
	require.NotContains(t, args, "--layer")
}

func TestBuildAxicliArgsLayersModeUsesLayerModeAndNumber(t *testing.T) {
	cfg := basePreset()
	device := deviceConfigView{Model: 1, Penlift: 1}
	layer := 5

	args := buildAxicliArgs(axicliTarget{filePath: "f.svg", layerNumber: &layer}, "c.svg", cfg, device, "")

	require.Equal(t, []string{"f.svg", "--mode", "layers", "--layer", "5"}, args[:5])
}

func TestBuildAxicliArgsAlwaysIncludesCheckpointOutput(t *testing.T) {
	cfg := basePreset()
	device := deviceConfigView{Model: 1, Penlift: 1}

	args := buildAxicliArgs(axicliTarget{filePath: "f.svg"}, "checkpoints/9.svg", cfg, device, "")

	require.Contains(t, args, "-o")
	require.Contains(t, args, "checkpoints/9.svg")
}

func TestBuildAxicliArgsResumeUsesResPlotModeIgnoringLayerNumber(t *testing.T) {
	cfg := basePreset()
	device := deviceConfigView{Model: 1, Penlift: 1}
	layer := 2

	target := axicliTarget{filePath: "checkpoints/9.svg", layerNumber: &layer, resume: true}
	args := buildAxicliArgs(target, "checkpoints/9.svg", cfg, device, "")

	require.Equal(t, []string{"checkpoints/9.svg", "--mode", "res_plot"}, args[:3])
	require.NotContains(t, args, "--layer")
}

// submitJob POSTs a Job-creation form to fileID's upload-scoped endpoint
// (POST /uploads/{id}/jobs) — the print page's route (ADR-0017) — without a
// file_id form field, since the URL itself scopes the request to fileID.
func submitJob(t *testing.T, s *Server, fileID int64, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return doForm(t, s, http.MethodPost, "/uploads/"+strconv.FormatInt(fileID, 10)+"/jobs", form)
}

var printJobIDPattern = regexp.MustCompile(`id="print-job-(\d+)"`)

// firstPrintJobID extracts a Job's id from a print page's rendered status
// card (see print_job_status) — the shape a Job-creation response now takes
// (ADR-0017).
func firstPrintJobID(t *testing.T, body string) int64 {
	t.Helper()
	match := printJobIDPattern.FindStringSubmatch(body)
	require.NotNil(t, match, "expected a print job status id in body: %s", body)
	id, err := strconv.ParseInt(match[1], 10, 64)
	require.NoError(t, err)
	return id
}

// seedFileAndPreset uploads a valid SVG and creates a Preset through the
// app's own endpoints, returning their ids for use in a job submission.
func seedFileAndPreset(t *testing.T, s *Server) (fileID, presetID int64) {
	t.Helper()
	return seedFileContentAndPreset(t, s, validSVG)
}

// layeredSVG has two AxiDraw-numbered layers, per its numeric-prefix
// layer-naming convention, for use in layers-mode Job tests.
const layeredSVG = `<svg xmlns="http://www.w3.org/2000/svg" xmlns:inkscape="http://www.inkscape.org/namespaces/inkscape" width="10" height="10">
  <g inkscape:groupmode="layer" inkscape:label="1 black"><rect width="5" height="5"/></g>
  <g inkscape:groupmode="layer" inkscape:label="2 red"><rect width="5" height="5" x="5" y="5"/></g>
</svg>`

// seedLayeredFileAndPreset uploads layeredSVG (two numbered layers) and
// creates a Preset, returning their ids for use in a layers-mode Job
// submission.
func seedLayeredFileAndPreset(t *testing.T, s *Server) (fileID, presetID int64) {
	t.Helper()
	return seedFileContentAndPreset(t, s, layeredSVG)
}

func seedFileContentAndPreset(t *testing.T, s *Server, svgContent string) (fileID, presetID int64) {
	t.Helper()

	rr := doMultipartUpload(t, s, "drawing.svg", []byte(svgContent))
	require.Equal(t, http.StatusOK, rr.Code)
	fileID = firstUploadID(t, rr.Body.String())

	rr = doForm(t, s, http.MethodPost, "/presets", presetForm("fine-detail"))
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)
	presetID = firstPresetID(t, rr.Body.String())

	return fileID, presetID
}

func jobRow(t *testing.T, s *Server, jobID int64) string {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs/"+strconv.FormatInt(jobID, 10)+"/row", nil)
	s.ServeHTTP(rr, req)
	return rr.Body.String()
}

func TestJobsListEmpty(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestJobsPageOffersNoWayToCreateAJob(t *testing.T) {
	s := newTestServer(t)
	seedFileAndPreset(t, s) // an Upload/Preset exist, so an absent form isn't just "nothing to pick from"

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	body := rr.Body.String()
	require.NotContains(t, body, `hx-post="/jobs"`, "job creation must have moved entirely to the print page")
	require.NotContains(t, body, `name="file_id"`)
	require.NotContains(t, body, "Select a file")
}

func TestPostJobsRouteNoLongerExists(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusMethodNotAllowed, rr.Code, "job creation is now exclusively /uploads/{id}/jobs; GET /jobs still exists for history")
}

func TestJobSubmitAndComplete(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	argsCh := make(chan []string, 1)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("plot complete"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "fine-detail")
	jobID := firstPrintJobID(t, rr.Body.String())

	var gotArgs []string
	select {
	case gotArgs = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	require.Contains(t, gotArgs, "--speed_pendown")
	require.Contains(t, gotArgs, "25") // preset's default, no override supplied

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)
}

func TestJobSubmitAppliesPassOverride(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	argsCh := make(chan []string, 1)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id":     {strconv.FormatInt(presetID, 10)},
		"speed_pendown": {"5"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	select {
	case gotArgs := <-argsCh:
		require.Contains(t, gotArgs, "5")
		require.NotContains(t, gotArgs, "25", "the preset's un-overridden default must not be used")
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond, "must wait for the background goroutine to finish before the test tears down its FileStore temp dir")
}

func TestJobRetryAfterFailureRunsFresh(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	var callCount int32
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		if atomic.AddInt32(&callCount, 1) == 1 {
			return []byte("device not found"), errors.New("exit status 1")
		}
		return []byte("plot complete"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "failed")
	}, 2*time.Second, 10*time.Millisecond)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/retry", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)

	require.EqualValues(t, 2, atomic.LoadInt32(&callCount))
}

func TestJobFailureDeletesCheckpoint(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	argsCh := make(chan []string, 1)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("device not found"), errors.New("exit status 1")
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	var args []string
	select {
	case args = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	checkpointPath := checkpointPathFromArgs(t, args)

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "failed")
	}, 2*time.Second, 10*time.Millisecond)

	_, err := os.Stat(checkpointPath)
	require.True(t, os.IsNotExist(err), "a genuinely failed pass must leave no checkpoint behind")
}

func TestJobRetryRejectedWhenNotFailed(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/retry", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusConflict, rr.Code)
}

func TestJobSubmitRejectedWhileAlreadyPrinting(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	started := make(chan struct{})
	release := make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}

	form := url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	}

	rr := submitJob(t, s, fileID, form)
	require.Equal(t, http.StatusOK, rr.Code)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	rr = submitJob(t, s, fileID, form)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "already printing")

	close(release)

	require.Eventually(t, func() bool {
		rr = httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		s.ServeHTTP(rr, req)
		return strings.Count(rr.Body.String(), `<tr id="job-`) == 1
	}, 2*time.Second, 10*time.Millisecond, "the rejected submission must not have created a second job")
}

func TestJobSubmitToUnknownUploadReturnsNotFound(t *testing.T) {
	s := newTestServer(t)
	_, presetID := seedFileAndPreset(t, s)

	rr := submitJob(t, s, 999, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestJobSubmitRejectsUnknownPreset(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedFileAndPreset(t, s)

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {"999"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "preset not found")
}

func TestDeriveJobStatusQueuedWhenFirstPassPending(t *testing.T) {
	status := deriveJobStatus([]passSummary{{SequenceIndex: 0, Status: "pending"}})
	require.Equal(t, "queued", status)
}

func TestDeriveJobStatusPrintingWhenAPassIsRunning(t *testing.T) {
	status := deriveJobStatus([]passSummary{
		{SequenceIndex: 0, Status: "complete"},
		{SequenceIndex: 1, Status: "running"},
	})
	require.Equal(t, "printing", status)
}

func TestDeriveJobStatusAwaitingNextPassWhenPriorPassesCompleteAndNextIsPending(t *testing.T) {
	status := deriveJobStatus([]passSummary{
		{SequenceIndex: 0, Status: "complete"},
		{SequenceIndex: 1, Status: "pending"},
	})
	require.Equal(t, "awaiting-next-pass", status)
}

func TestDeriveJobStatusCompleteWhenAllPassesComplete(t *testing.T) {
	status := deriveJobStatus([]passSummary{
		{SequenceIndex: 0, Status: "complete"},
		{SequenceIndex: 1, Status: "complete"},
	})
	require.Equal(t, "complete", status)
}

func TestDeriveJobStatusFailedSurfacesAsIs(t *testing.T) {
	status := deriveJobStatus([]passSummary{
		{SequenceIndex: 0, Status: "complete"},
		{SequenceIndex: 1, Status: "failed"},
	})
	require.Equal(t, "failed", status)
}

func TestActivePassReturnsFirstIncompletePass(t *testing.T) {
	passes := []passSummary{
		{ID: 1, SequenceIndex: 0, Status: "complete"},
		{ID: 2, SequenceIndex: 1, Status: "pending"},
		{ID: 3, SequenceIndex: 2, Status: "pending"},
	}
	require.Equal(t, int64(2), activePass(passes).ID)
}

func TestActivePassReturnsLastPassWhenAllComplete(t *testing.T) {
	passes := []passSummary{
		{ID: 1, SequenceIndex: 0, Status: "complete"},
		{ID: 2, SequenceIndex: 1, Status: "complete"},
	}
	require.Equal(t, int64(2), activePass(passes).ID)
}

func intPtr(n int) *int { return &n }

func TestBuildJobRowViewSetsLayerNumberAndPrevLayerNumberWhenAwaitingNextPass(t *testing.T) {
	passes := []passSummary{
		{ID: 1, SequenceIndex: 0, Status: "complete", LayerNumber: intPtr(1)},
		{ID: 2, SequenceIndex: 1, Status: "pending", LayerNumber: intPtr(2)},
	}
	row := buildJobRowView(10, 20, "drawing.svg", "2026-01-01T00:00:00Z", passes)

	require.Equal(t, "awaiting-next-pass", row.Status)
	require.NotNil(t, row.LayerNumber)
	require.Equal(t, 2, *row.LayerNumber, "the active (not-yet-run) Pass's own layer number")
	require.NotNil(t, row.PrevLayerNumber)
	require.Equal(t, 1, *row.PrevLayerNumber, "the immediately preceding, already-complete Pass's layer number")
}

func TestBuildJobRowViewLeavesPrevLayerNumberNilForTheFirstPass(t *testing.T) {
	passes := []passSummary{
		{ID: 1, SequenceIndex: 0, Status: "pending", LayerNumber: intPtr(1)},
	}
	row := buildJobRowView(10, 20, "drawing.svg", "2026-01-01T00:00:00Z", passes)

	require.NotNil(t, row.LayerNumber)
	require.Nil(t, row.PrevLayerNumber, "there is no preceding Pass to report")
}

func TestBuildJobRowViewLeavesLayerNumberNilForWholeModeJobs(t *testing.T) {
	passes := []passSummary{
		{ID: 1, SequenceIndex: 0, Status: "pending", LayerNumber: nil},
	}
	row := buildJobRowView(10, 20, "drawing.svg", "2026-01-01T00:00:00Z", passes)

	require.Nil(t, row.LayerNumber)
	require.Nil(t, row.PrevLayerNumber)
}

func TestJobSubmitLayersModeCreatesOnePassPerLayerAndStartsFirst(t *testing.T) {
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
	jobID := firstPrintJobID(t, rr.Body.String())

	var gotArgs []string
	select {
	case gotArgs = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	require.Contains(t, gotArgs, "layers")
	require.Contains(t, gotArgs, "--layer")
	require.Contains(t, gotArgs, "1", "the lowest-numbered layer must run first")

	require.Eventually(t, func() bool {
		body := jobRow(t, s, jobID)
		// Once layer 1 completes, the active/reported pass becomes layer 2
		// (pending) — "2/2" — but the job must sit in awaiting-next-pass
		// rather than actually starting it until the user triggers advance.
		return strings.Contains(body, "2/2") && strings.Contains(body, "awaiting-next-pass")
	}, 2*time.Second, 10*time.Millisecond, "job must wait for an explicit advance, not auto-continue to layer 2")

	select {
	case <-argsCh:
		t.Fatal("axicli must not be invoked again before an explicit advance")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestJobLayersModeAdvanceRunsNextLayerThenCompletes(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)

	argsCh := make(chan []string, 2)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("layer plotted"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	select {
	case <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked for layer 1")
	}

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "awaiting-next-pass")
	}, 2*time.Second, 10*time.Millisecond)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/advance", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var secondArgs []string
	select {
	case secondArgs = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked for layer 2")
	}
	require.Contains(t, secondArgs, "2")

	require.Eventually(t, func() bool {
		body := jobRow(t, s, jobID)
		return strings.Contains(body, "complete") && strings.Contains(body, "2/2")
	}, 2*time.Second, 10*time.Millisecond, "job must reach complete only after its final pass completes")
}

func TestJobLayersModeAdvanceRejectedBeforeAwaitingNextPass(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)

	started := make(chan struct{})
	release := make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/advance", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusConflict, rr.Code)

	close(release)
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "awaiting-next-pass")
	}, 2*time.Second, 10*time.Millisecond, "must wait for the background goroutine to finish before the test tears down its FileStore temp dir")
}

// submitLayersJobAwaitingNextPass submits a layers-mode Job against
// seedLayeredFileAndPreset's two layers ("1 black", "2 red"), waiting for
// layer 1 to complete so the Job sits in awaiting-next-pass — the state
// axicontrol-layers-pause-ui's print-page context is shown in.
func submitLayersJobAwaitingNextPass(t *testing.T, s *Server, fileID, presetID int64) (jobID int64) {
	t.Helper()
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("layer plotted"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID = firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "awaiting-next-pass")
	}, 2*time.Second, 10*time.Millisecond)
	return jobID
}

func TestPrintPageShowsFinishedAndNextLayerWhileAwaitingNextPass(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s) // layers 1 (black), 2 (red)
	jobID := submitLayersJobAwaitingNextPass(t, s, fileID, presetID)

	body := printPage(t, s, fileID).Body.String()
	require.Contains(t, body, "1 of 2 done")
	require.Contains(t, body, "Finished layer 1")
	require.Contains(t, body, "black")
	require.Contains(t, body, "Next: layer 2")
	require.Contains(t, body, "red")
	require.Contains(t, body, "/uploads/"+itoa(fileID)+"/layers/2/content", "the next layer's live preview must use the isolated-layer endpoint")
	require.Contains(t, body, `hx-post="/jobs/`+itoa(jobID)+`/advance"`)
	require.Contains(t, body, `hx-target="#print-job-`+itoa(jobID)+`"`)
}

func TestPrintPageShowsBarePassCounterForNonPausedStatuses(t *testing.T) {
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
	require.Contains(t, body, "Pass 1/1")
	require.NotContains(t, body, "of 1 done", "the layers-pause layout is only for awaiting-next-pass")
}

func TestJobsRowUnaffectedByLayersPauseUI(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)
	jobID := submitLayersJobAwaitingNextPass(t, s, fileID, presetID)

	body := jobRow(t, s, jobID)
	require.Contains(t, body, "2/2", "/jobs keeps its plain Pass N/M display")
	require.NotContains(t, body, "Finished layer", "/jobs must not gain the print page's layer-label context")
	require.NotContains(t, body, "black")
}

func TestAdvanceFromPrintPageRendersPrintJobStatusFragment(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)
	jobID := submitLayersJobAwaitingNextPass(t, s, fileID, presetID)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/advance", nil)
	req.Header.Set("HX-Target", "print-job-"+strconv.FormatInt(jobID, 10))
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	require.Contains(t, body, `id="print-job-`+strconv.FormatInt(jobID, 10)+`"`)
	require.NotContains(t, body, `id="job-`+strconv.FormatInt(jobID, 10)+`"`, "a request targeting the print page's own card must not get /jobs's row fragment back")
}

func TestJobLayersModeFailedPassCanBeRetriedThenAdvanced(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)

	var callCount int32
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		if atomic.AddInt32(&callCount, 1) == 1 {
			return []byte("device error"), errors.New("exit status 1")
		}
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "failed")
	}, 2*time.Second, 10*time.Millisecond)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/retry", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "awaiting-next-pass")
	}, 2*time.Second, 10*time.Millisecond, "retrying the failed first layer must not skip the second")

	require.EqualValues(t, 2, atomic.LoadInt32(&callCount))
}

func TestJobSubmitLayersModeRejectsSVGWithNoNumberedLayers(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s) // validSVG has no layers at all

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "no numbered layers")
}

func TestJobSubmitSingleLayerModeCreatesOnePassForChosenLayerOnly(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s) // layers 1 and 2

	argsCh := make(chan []string, 2)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("layer plotted"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id":    {strconv.FormatInt(presetID, 10)},
		"mode":         {"single_layer"},
		"layer_number": {"2"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	var gotArgs []string
	select {
	case gotArgs = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	require.Contains(t, gotArgs, "layers")
	require.Contains(t, gotArgs, "--layer")
	require.Contains(t, gotArgs, "2")

	require.Eventually(t, func() bool {
		body := jobRow(t, s, jobID)
		return strings.Contains(body, "complete") && strings.Contains(body, "1/1")
	}, 2*time.Second, 10*time.Millisecond, "a Single Layer Job has exactly one Pass and must reach complete on its own")

	select {
	case <-argsCh:
		t.Fatal("axicli must not be invoked again: no other layer is touched")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestJobSubmitSingleLayerModeRejectsMissingLayerNumber(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"single_layer"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "select a layer")
}

func TestJobSubmitSingleLayerModeRejectsUnknownLayerNumber(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id":    {strconv.FormatInt(presetID, 10)},
		"mode":         {"single_layer"},
		"layer_number": {"99"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "layer not found")
}

func TestJobLayersModeHoldsDeviceClaimThroughAwaitingNextPass(t *testing.T) {
	s := newTestServer(t)
	layeredFileID, presetID := seedLayeredFileAndPreset(t, s)
	wholeFileID, _ := seedFileAndPreset(t, s)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, layeredFileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "awaiting-next-pass")
	}, 2*time.Second, 10*time.Millisecond)

	// A different, unrelated Job must not be able to start on the AxiDraw
	// while the first Job's artwork is still mounted mid-sequence, waiting
	// on its own next-layer trigger.
	rr = submitJob(t, s, wholeFileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"whole"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "already printing")

	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/advance", nil)
	rr = httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)

	// Now that the layered Job is fully done, the device is free again.
	rr = submitJob(t, s, wholeFileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"whole"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotContains(t, rr.Body.String(), "already printing")
	thirdJobID := firstPrintJobID(t, rr.Body.String())
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, thirdJobID), "complete")
	}, 2*time.Second, 10*time.Millisecond, "must wait for the third job's own background goroutine to finish before the test tears down its FileStore temp dir")
}

func TestJobSubmitRejectsInvalidMode(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"bogus"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "invalid mode")
}

// postJob issues a bodyless POST to one of a Job's action endpoints
// (pause/resume/cancel/retry/advance), the shape all of them share.
func postJob(t *testing.T, s *Server, jobID int64, action string) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/jobs/"+strconv.FormatInt(jobID, 10)+"/"+action, nil)
	s.ServeHTTP(rr, req)
	return rr
}

// checkpointPathFromArgs extracts the -o flag's value from a captured
// axicli invocation.
func checkpointPathFromArgs(t *testing.T, args []string) string {
	t.Helper()
	for i, a := range args {
		if a == "-o" && i+1 < len(args) {
			return args[i+1]
		}
	}
	t.Fatalf("no -o flag in axicli args: %v", args)
	return ""
}

// interruptibleAxicli returns a fake s.runAxicli plus a channel reporting
// each invocation's args: it blocks until ctx is canceled (simulating
// requestInterrupt's SIGINT), then writes progress to the -o path — the way
// real axicli checkpoints on a clean pause — and returns an error, as if
// axicli exited nonzero on interrupt.
func interruptibleAxicli() (fn func(ctx context.Context, args ...string) ([]byte, error), argsCh chan []string) {
	argsCh = make(chan []string, 8)
	fn = func(ctx context.Context, args ...string) ([]byte, error) {
		argsCh <- args
		<-ctx.Done()
		for i, a := range args {
			if a == "-o" && i+1 < len(args) {
				_ = os.WriteFile(args[i+1], []byte("progress"), 0o644)
			}
		}
		return []byte("interrupted"), errors.New("signal: interrupt")
	}
	return fn, argsCh
}

func TestJobPauseThenResumeCompletesFromCheckpoint(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	var callCount int32
	fakeRun, argsCh := interruptibleAxicli()
	s.runAxicli = func(ctx context.Context, args ...string) ([]byte, error) {
		if atomic.AddInt32(&callCount, 1) == 1 {
			return fakeRun(ctx, args...)
		}
		argsCh <- args
		return []byte("plot complete"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	var firstArgs []string
	select {
	case firstArgs = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	checkpointPath := checkpointPathFromArgs(t, firstArgs)

	rr = postJob(t, s, jobID, "pause")
	require.Equal(t, http.StatusOK, rr.Code)

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "paused")
	}, 2*time.Second, 10*time.Millisecond)

	_, err := os.Stat(checkpointPath)
	require.NoError(t, err, "a pause must leave a checkpoint file behind")

	rr = postJob(t, s, jobID, "resume")
	require.Equal(t, http.StatusOK, rr.Code)

	var resumeArgs []string
	select {
	case resumeArgs = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked for resume")
	}
	require.Equal(t, checkpointPath, resumeArgs[0], "resume must plot the checkpoint file, not the original")
	require.Contains(t, resumeArgs, "res_plot")
	require.Equal(t, checkpointPath, checkpointPathFromArgs(t, resumeArgs), "resume must keep writing to the same checkpoint key")

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)
	require.EqualValues(t, 2, atomic.LoadInt32(&callCount))

	_, err = os.Stat(checkpointPath)
	require.True(t, os.IsNotExist(err), "the checkpoint must be gone once the Pass completes")
}

func TestJobCanBePausedAndResumedMoreThanOnce(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	var callCount int32
	fakeRun, argsCh := interruptibleAxicli()
	s.runAxicli = func(ctx context.Context, args ...string) ([]byte, error) {
		if atomic.AddInt32(&callCount, 1) <= 2 {
			return fakeRun(ctx, args...)
		}
		argsCh <- args
		return []byte("plot complete"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	for i := 0; i < 2; i++ {
		select {
		case <-argsCh:
		case <-time.After(2 * time.Second):
			t.Fatalf("axicli was not invoked for run %d", i+1)
		}

		rr = postJob(t, s, jobID, "pause")
		require.Equal(t, http.StatusOK, rr.Code)
		require.Eventually(t, func() bool {
			return strings.Contains(jobRow(t, s, jobID), "paused")
		}, 2*time.Second, 10*time.Millisecond, "pause/resume cycle %d", i+1)

		rr = postJob(t, s, jobID, "resume")
		require.Equal(t, http.StatusOK, rr.Code)
	}

	select {
	case <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked for final run")
	}

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)
	require.EqualValues(t, 3, atomic.LoadInt32(&callCount), "must have run fresh once, then resumed twice")
}

func TestJobPauseRejectedWhenNotPrinting(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)

	rr = postJob(t, s, jobID, "pause")
	require.Equal(t, http.StatusConflict, rr.Code)
}

func TestJobResumeRejectedWhenNotPaused(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)

	rr = postJob(t, s, jobID, "resume")
	require.Equal(t, http.StatusConflict, rr.Code)
}

func TestJobCancelWhileRunningInterruptsAndLandsCancelled(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	fakeRun, argsCh := interruptibleAxicli()
	s.runAxicli = fakeRun

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	var args []string
	select {
	case args = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	checkpointPath := checkpointPathFromArgs(t, args)

	rr = postJob(t, s, jobID, "cancel")
	require.Equal(t, http.StatusOK, rr.Code)

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "cancelled")
	}, 2*time.Second, 10*time.Millisecond)
	require.NotContains(t, jobRow(t, s, jobID), "paused")

	_, err := os.Stat(checkpointPath)
	require.True(t, os.IsNotExist(err), "cancelling a running pass must leave no checkpoint behind")

	// The device must be free again for a new job. Swap in a fast fake
	// first — the original one blocks on ctx.Done(), which nothing here
	// would ever cancel.
	s.runAxicli = func(_ context.Context, _ ...string) ([]byte, error) {
		return []byte("ok"), nil
	}
	rr = submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotContains(t, rr.Body.String(), "already printing")
	newJobID := firstPrintJobID(t, rr.Body.String())
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, newJobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)
}

func TestJobCancelFromPausedLeavesNoCheckpointAndJobCancelled(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	fakeRun, argsCh := interruptibleAxicli()
	s.runAxicli = fakeRun

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	var args []string
	select {
	case args = <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
	checkpointPath := checkpointPathFromArgs(t, args)

	rr = postJob(t, s, jobID, "pause")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "paused")
	}, 2*time.Second, 10*time.Millisecond)

	_, err := os.Stat(checkpointPath)
	require.NoError(t, err, "pause must have left a checkpoint")

	rr = postJob(t, s, jobID, "cancel")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, jobRow(t, s, jobID), "cancelled")

	_, err = os.Stat(checkpointPath)
	require.True(t, os.IsNotExist(err), "cancelling a paused pass must leave no checkpoint behind")
}

func TestJobCancelFromAwaitingNextPassCancelsSynchronously(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)

	s.runAxicli = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("layer plotted"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "awaiting-next-pass")
	}, 2*time.Second, 10*time.Millisecond)

	rr = postJob(t, s, jobID, "cancel")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, jobRow(t, s, jobID), "cancelled")

	// The device must be free again for a new job.
	rr = submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"whole"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.NotContains(t, rr.Body.String(), "already printing")
	newJobID := firstPrintJobID(t, rr.Body.String())
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, newJobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)
}

func TestJobCancelRejectedWhenAlreadyTerminal(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(ctx context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "complete")
	}, 2*time.Second, 10*time.Millisecond)

	rr = postJob(t, s, jobID, "cancel")
	require.Equal(t, http.StatusConflict, rr.Code)
}

func passStatus(t *testing.T, s *Server, passID int64) string {
	t.Helper()
	var status string
	require.NoError(t, s.db.QueryRowContext(context.Background(), "SELECT status FROM passes WHERE id = ?", passID).Scan(&status))
	return status
}

func TestTryStartPassRunFailsIfStatusChangedConcurrently(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	ctx := context.Background()

	passID, err := s.insertJobAndPasses(ctx, fileID, presetID, "whole", nil, overrides{})
	require.NoError(t, err)

	// Simulate a concurrent synchronous cancel landing between executePass
	// reading "pending" and its attempt to transition to "running".
	require.NoError(t, s.setPassStatus(ctx, passID, "cancelled", ""))

	started, err := s.tryStartPassRun(ctx, passID, "pending")
	require.NoError(t, err)
	require.False(t, started, "must not clobber a status that changed since it was read")
	require.Equal(t, "cancelled", passStatus(t, s, passID))
}

func TestTryStartPassRunSucceedsWhenStatusMatches(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	ctx := context.Background()

	passID, err := s.insertJobAndPasses(ctx, fileID, presetID, "whole", nil, overrides{})
	require.NoError(t, err)

	started, err := s.tryStartPassRun(ctx, passID, "pending")
	require.NoError(t, err)
	require.True(t, started)
	require.Equal(t, "running", passStatus(t, s, passID))
}

func TestTryCancelIfNotRunningFailsWhenAlreadyRunning(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	ctx := context.Background()

	passID, err := s.insertJobAndPasses(ctx, fileID, presetID, "whole", nil, overrides{})
	require.NoError(t, err)
	require.NoError(t, s.setPassStatus(ctx, passID, "running", ""))

	cancelled, err := s.tryCancelIfNotRunning(ctx, passID)
	require.NoError(t, err)
	require.False(t, cancelled, "must not clobber a pass that started running since it was checked")
	require.Equal(t, "running", passStatus(t, s, passID))
}

func TestTryCancelIfNotRunningSucceedsWhenPendingOrPaused(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	ctx := context.Background()

	for _, status := range []string{"pending", "paused"} {
		passID, err := s.insertJobAndPasses(ctx, fileID, presetID, "whole", nil, overrides{})
		require.NoError(t, err)
		require.NoError(t, s.setPassStatus(ctx, passID, status, ""))

		cancelled, err := s.tryCancelIfNotRunning(ctx, passID)
		require.NoError(t, err, "status %q", status)
		require.True(t, cancelled, "status %q", status)
		require.Equal(t, "cancelled", passStatus(t, s, passID))
	}
}
