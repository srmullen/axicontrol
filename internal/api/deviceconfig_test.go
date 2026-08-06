package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/srmullen/axicontrol/internal/filestore"
	"github.com/srmullen/axicontrol/internal/store"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return NewServer(db, newTestFileStore(t), "", testLogger())
}

func newTestFileStore(t *testing.T) filestore.FileStore {
	t.Helper()
	fs, err := filestore.NewPVStore(t.TempDir())
	require.NoError(t, err)
	return fs
}

func TestDeviceConfigShowsDefaults(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/device-config", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()
	require.Contains(t, body, `value="1"`, "default model and penlift are both 1")
}

func TestDeviceConfigUpdatePersists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	db, err := store.Open(dbPath)
	require.NoError(t, err)

	s := NewServer(db, newTestFileStore(t), "", testLogger())

	form := url.Values{"model": {"3"}, "penlift": {"2"}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/device-config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `value="3"`)
	require.Contains(t, rr.Body.String(), `value="2"`)
	require.NoError(t, db.Close())

	// Simulate a process restart against the same db file.
	db2, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db2.Close()) })
	s2 := NewServer(db2, newTestFileStore(t), "", testLogger())

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/device-config", nil)
	s2.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), `value="3"`)
	require.Contains(t, rr.Body.String(), `value="2"`)
}

func TestDeviceConfigUpdateRejectsNonNumericInput(t *testing.T) {
	s := newTestServer(t)

	form := url.Values{"model": {"not-a-number"}, "penlift": {"1"}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/device-config", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.ServeHTTP(rr, req)

	// htmx only swaps a fragment into the DOM on a 2xx response by default,
	// so the inline error must ship as 200 for the user to actually see it.
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "must both be integers")
}
