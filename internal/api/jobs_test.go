package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
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

	args := buildAxicliArgs("/data/files/a.svg", cfg, device, "")

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
	}, args)
}

func TestBuildAxicliArgsWithDevicePath(t *testing.T) {
	cfg := basePreset()
	device := deviceConfigView{Model: 1, Penlift: 1}

	args := buildAxicliArgs("/data/files/a.svg", cfg, device, "/dev/axidraw")

	require.Equal(t, []string{"--port", "/dev/axidraw"}, args[len(args)-2:])
}

func TestBuildAxicliArgsConstSpeedFlagOnlyWhenTrue(t *testing.T) {
	device := deviceConfigView{Model: 1, Penlift: 1}

	off := basePreset()
	off.ConstSpeed = false
	require.NotContains(t, buildAxicliArgs("f.svg", off, device, ""), "--const_speed")

	on := basePreset()
	on.ConstSpeed = true
	require.Contains(t, buildAxicliArgs("f.svg", on, device, ""), "--const_speed")
}

func TestBuildAxicliArgsAppliesOverriddenSpeed(t *testing.T) {
	device := deviceConfigView{Model: 1, Penlift: 1}
	base := basePreset()
	overridden := 99
	resolved := (overrides{SpeedPendown: &overridden}).apply(base)

	args := buildAxicliArgs("f.svg", resolved, device, "")

	require.Contains(t, args, "99")
	require.NotContains(t, args, "25")
}

var jobRowIDPattern = regexp.MustCompile(`id="job-(\d+)"`)

func firstJobID(t *testing.T, body string) int64 {
	t.Helper()
	match := jobRowIDPattern.FindStringSubmatch(body)
	require.NotNil(t, match, "expected a job row id in body: %s", body)
	id, err := strconv.ParseInt(match[1], 10, 64)
	require.NoError(t, err)
	return id
}

// seedFileAndPreset uploads a valid SVG and creates a Preset through the
// app's own endpoints, returning their ids for use in a job submission.
func seedFileAndPreset(t *testing.T, s *Server) (fileID, presetID int64) {
	t.Helper()

	rr := doMultipartUpload(t, s, "drawing.svg", []byte(validSVG))
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

func TestJobSubmitAndComplete(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	argsCh := make(chan []string, 1)
	s.runAxicli = func(args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("plot complete"), nil
	}

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "drawing.svg")
	jobID := firstJobID(t, rr.Body.String())

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
	s.runAxicli = func(args ...string) ([]byte, error) {
		argsCh <- args
		return []byte("ok"), nil
	}

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":       {strconv.FormatInt(fileID, 10)},
		"preset_id":     {strconv.FormatInt(presetID, 10)},
		"speed_pendown": {"5"},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	select {
	case gotArgs := <-argsCh:
		require.Contains(t, gotArgs, "5")
		require.NotContains(t, gotArgs, "25", "the preset's un-overridden default must not be used")
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}
}

func TestJobRetryAfterFailureRunsFresh(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	var callCount int32
	s.runAxicli = func(args ...string) ([]byte, error) {
		if atomic.AddInt32(&callCount, 1) == 1 {
			return []byte("device not found"), errors.New("exit status 1")
		}
		return []byte("plot complete"), nil
	}

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstJobID(t, rr.Body.String())

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

func TestJobRetryRejectedWhenNotFailed(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstJobID(t, rr.Body.String())

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
	s.runAxicli = func(args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}

	form := url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	}

	rr := doForm(t, s, http.MethodPost, "/jobs", form)
	require.Equal(t, http.StatusOK, rr.Code)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	rr = doForm(t, s, http.MethodPost, "/jobs", form)
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

func TestJobSubmitRejectsUnknownFile(t *testing.T) {
	s := newTestServer(t)
	_, presetID := seedFileAndPreset(t, s)

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {"999"},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "file not found")
}

func TestJobSubmitRejectsUnknownPreset(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedFileAndPreset(t, s)

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {"999"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "preset not found")
}
