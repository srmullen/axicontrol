package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/srmullen/axicontrol/internal/store"
)

var presetRowIDPattern = regexp.MustCompile(`id="preset-(\d+)"`)

func firstPresetID(t *testing.T, body string) int64 {
	t.Helper()
	match := presetRowIDPattern.FindStringSubmatch(body)
	require.NotNil(t, match, "expected a preset row id in body: %s", body)
	id, err := strconv.ParseInt(match[1], 10, 64)
	require.NoError(t, err)
	return id
}

func itoa(id int64) string {
	return strconv.FormatInt(id, 10)
}

func presetForm(name string) url.Values {
	return url.Values{
		"name":           {name},
		"description":    {"a test preset"},
		"speed_pendown":  {"25"},
		"speed_penup":    {"75"},
		"accel":          {"75"},
		"pen_pos_down":   {"40"},
		"pen_pos_up":     {"60"},
		"pen_rate_lower": {"50"},
		"pen_rate_raise": {"50"},
		"pen_delay_down": {"0"},
		"pen_delay_up":   {"0"},
	}
}

func doForm(t *testing.T, s *Server, method, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.ServeHTTP(rr, req)
	return rr
}

func TestPresetsListEmpty(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestPresetCreateThenList(t *testing.T) {
	s := newTestServer(t)

	rr := doForm(t, s, http.MethodPost, "/presets", presetForm("draft"))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "draft")

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "draft")
}

func TestPresetCreateDuplicateNameRejected(t *testing.T) {
	s := newTestServer(t)

	rr := doForm(t, s, http.MethodPost, "/presets", presetForm("draft"))
	require.Equal(t, http.StatusOK, rr.Code)

	rr = doForm(t, s, http.MethodPost, "/presets", presetForm("draft"))
	// htmx only swaps a fragment into the DOM on a 2xx response by default,
	// so the inline conflict error must ship as 200 for the user to see it.
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "already exists")

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, 1, strings.Count(rr.Body.String(), `<tr id="preset-`), "duplicate create must not add a second row")
}

func TestPresetUpdate(t *testing.T) {
	s := newTestServer(t)

	rr := doForm(t, s, http.MethodPost, "/presets", presetForm("draft"))
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)
	id := firstPresetID(t, rr.Body.String())
	require.NotZero(t, id)

	updated := presetForm("draft-renamed")
	rr = doForm(t, s, http.MethodPut, "/presets/"+itoa(id), updated)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "draft-renamed")
}

func TestPresetUpdateDuplicateNameRejected(t *testing.T) {
	s := newTestServer(t)

	require.Equal(t, http.StatusOK, doForm(t, s, http.MethodPost, "/presets", presetForm("draft")).Code)
	require.Equal(t, http.StatusOK, doForm(t, s, http.MethodPost, "/presets", presetForm("fine-detail")).Code)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)
	id := firstPresetID(t, rr.Body.String())

	// Renaming "draft" (whichever preset sorts first) to the other's name
	// must be rejected without silently overwriting it.
	rr = doForm(t, s, http.MethodPut, "/presets/"+itoa(id), presetForm("fine-detail"))
	// htmx only swaps a fragment into the DOM on a 2xx response by default,
	// so the inline conflict error must ship as 200 for the user to see it.
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "already exists")
}

func TestPresetDelete(t *testing.T) {
	s := newTestServer(t)

	rr := doForm(t, s, http.MethodPost, "/presets", presetForm("draft"))
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)
	id := firstPresetID(t, rr.Body.String())
	require.NotZero(t, id)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/presets/"+itoa(id), nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/presets", nil)
	s.ServeHTTP(rr, req)
	require.NotContains(t, rr.Body.String(), "draft")
}

func TestPresetsPersistAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	s := NewServer(db, newTestFileStore(t), "", testLogger())

	rr := doForm(t, s, http.MethodPost, "/presets", presetForm("draft"))
	require.Equal(t, http.StatusOK, rr.Code)
	require.NoError(t, db.Close())

	db2, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db2.Close()) })
	s2 := NewServer(db2, newTestFileStore(t), "", testLogger())

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/presets", nil)
	s2.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "draft")
}
