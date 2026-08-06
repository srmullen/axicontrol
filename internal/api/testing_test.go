package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/srmullen/axicontrol/internal/store"
)

func TestTestingPageRenders(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/testing", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "Tracked carriage position: (0, 0)")
}

func postTest(t *testing.T, s *Server, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	return doForm(t, s, http.MethodPost, path, form)
}

func TestTestSysinfoInvokesAxicliAndShowsOutput(t *testing.T) {
	s := newTestServer(t)
	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("model: AxiDraw V3"), nil
	}

	rr := postTest(t, s, "/testing/sysinfo", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "model: AxiDraw V3")
	require.Equal(t, []string{"sysinfo"}, gotArgs)
}

func TestTestCycleUsesCycleMode(t *testing.T) {
	s := newTestServer(t)
	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("ok"), nil
	}

	rr := postTest(t, s, "/testing/cycle", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, []string{"--mode", "cycle"}, gotArgs)
}

func TestTestToggleUsesToggleMode(t *testing.T) {
	s := newTestServer(t)
	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("ok"), nil
	}

	rr := postTest(t, s, "/testing/toggle", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, []string{"--mode", "toggle"}, gotArgs)
}

func TestTestAlignUsesAlignMode(t *testing.T) {
	s := newTestServer(t)
	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("ok"), nil
	}

	rr := postTest(t, s, "/testing/align", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, []string{"--mode", "align"}, gotArgs)
}

func TestTestActionsPassDevicePath(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s := NewServer(db, newTestFileStore(t), "/dev/axidraw", testLogger())

	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("ok"), nil
	}

	rr := postTest(t, s, "/testing/align", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, []string{"--mode", "align", "--port", "/dev/axidraw"}, gotArgs)
}

func TestTestWalkHomeResetsTrackedPosition(t *testing.T) {
	s := newTestServer(t)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	// Move away from (0, 0) first, then home, and confirm the tracked
	// position display goes back to (0, 0).
	rr := postTest(t, s, "/testing/move", url.Values{"x": {"3"}, "y": {"4"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "(3, 4)")

	var gotArgs []string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("homed"), nil
	}
	rr = postTest(t, s, "/testing/walk-home", nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "(0, 0)")
	require.Equal(t, []string{"--mode", "manual", "--manual_cmd", "walk_home"}, gotArgs)
}

func TestTestMoveIssuesRelativeWalkDeltasFromTrackedPosition(t *testing.T) {
	s := newTestServer(t)
	var allArgs [][]string
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		allArgs = append(allArgs, args)
		return []byte("ok"), nil
	}

	rr := postTest(t, s, "/testing/move", url.Values{"x": {"2"}, "y": {"-1"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, allArgs, 2, "first move from (0,0) issues both a walk_x and a walk_y")
	require.Equal(t, []string{"--mode", "manual", "--manual_cmd", "walk_x", "--dist", "2"}, allArgs[0])
	require.Equal(t, []string{"--mode", "manual", "--manual_cmd", "walk_y", "--dist", "-1"}, allArgs[1])

	allArgs = nil
	rr = postTest(t, s, "/testing/move", url.Values{"x": {"5"}, "y": {"-1"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Len(t, allArgs, 1, "unchanged y must not issue a second walk_y")
	require.Equal(t, []string{"--mode", "manual", "--manual_cmd", "walk_x", "--dist", "3"}, allArgs[0],
		"delta must be computed from the previously tracked position, not from zero")
}

func TestTestMoveRejectsNonNumericCoordinates(t *testing.T) {
	s := newTestServer(t)
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := postTest(t, s, "/testing/move", url.Values{"x": {"nope"}, "y": {"1"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "x must be a number")
}

func TestTestMoveStopsBeforeYOnXFailureAndLeavesPositionUnchanged(t *testing.T) {
	s := newTestServer(t)
	var calls int
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		calls++
		return []byte("motor fault"), errors.New("exit status 1")
	}

	rr := postTest(t, s, "/testing/move", url.Values{"x": {"2"}, "y": {"3"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "move failed")
	require.Equal(t, 1, calls, "a failed walk_x must not be followed by walk_y")
	require.Contains(t, rr.Body.String(), "(0, 0)", "a failed move must not update tracked position")
}

func TestTestActionsRejectedWhileJobPrinting(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	started := make(chan struct{})
	release := make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}

	rr := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	<-started
	defer close(release)

	for _, path := range []string{"/testing/sysinfo", "/testing/cycle", "/testing/toggle", "/testing/align", "/testing/walk-home"} {
		rr := postTest(t, s, path, nil)
		require.Equal(t, http.StatusOK, rr.Code, path)
		require.Contains(t, rr.Body.String(), "already printing", path)
	}

	rr = postTest(t, s, "/testing/move", url.Values{"x": {"1"}, "y": {"1"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "already printing")
}

func TestTestActionClaimsDeviceSoAJobCannotStartConcurrently(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	started := make(chan struct{})
	release := make(chan struct{})
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		close(started)
		<-release
		return []byte("ok"), nil
	}

	// The HTTP handler blocks until the jog action's axicli call returns, so
	// it has to run in the background; synchronize on `started` instead of
	// on the (not-yet-available) response. postTest/doForm make no `t`
	// assertions themselves, so calling them off the test goroutine is safe.
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		done <- postTest(t, s, "/testing/cycle", nil)
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("cycle never invoked axicli")
	}

	// A real Job submission must be rejected while the jog command's own
	// axicli call is still in flight — this is the race runTestAction's
	// tryClaimDevice/releaseDevice pairing (rather than a bare deviceClaimed
	// peek) exists to close.
	jobRR := doForm(t, s, http.MethodPost, "/jobs", url.Values{
		"file_id":   {strconv.FormatInt(fileID, 10)},
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, jobRR.Code)
	require.Contains(t, jobRR.Body.String(), "already printing")

	close(release)
	<-done
}

func TestTestPositionResetsAcrossServerRestart(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	s := NewServer(db, newTestFileStore(t), "", testLogger())
	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := postTest(t, s, "/testing/move", url.Values{"x": {"7"}, "y": {"8"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "(7, 8)")
	require.NoError(t, db.Close())

	// A pod restart never persists tracked position (ADR-0004) — a fresh
	// Server against the same db starts back at (0, 0).
	db2, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db2.Close()) })
	s2 := NewServer(db2, newTestFileStore(t), "", testLogger())

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/testing", nil)
	s2.ServeHTTP(rr, req)
	require.Contains(t, rr.Body.String(), "(0, 0)")
}
