package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/srmullen/axicontrol/internal/store"
)

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func TestHeartbeatCreateThenList(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	s := NewServer(db, "", testLogger())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/heartbeat", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var created heartbeat
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &created))
	require.NotZero(t, created.ID)
	require.NotEmpty(t, created.CreatedAt)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/heartbeat", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var list []heartbeat
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
	require.Len(t, list, 1)
	require.Equal(t, created.ID, list[0].ID)
}

func TestHeartbeatListEmpty(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	db, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	s := NewServer(db, "", testLogger())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/heartbeat", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var list []heartbeat
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
	require.Empty(t, list)
}

func TestHeartbeatPersistsAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")

	db1, err := store.Open(dbPath)
	require.NoError(t, err)
	s1 := NewServer(db1, "", testLogger())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/heartbeat", nil)
	s1.ServeHTTP(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)
	require.NoError(t, db1.Close())

	// Simulate a process restart against the same db file.
	db2, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db2.Close()) })
	s2 := NewServer(db2, "", testLogger())

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/heartbeat", nil)
	s2.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var list []heartbeat
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &list))
	require.Len(t, list, 1)
}
