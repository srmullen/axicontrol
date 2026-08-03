package store

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenAppliesMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "axicontrol.sqlite")

	db, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM heartbeats").Scan(&count)
	require.NoError(t, err, "heartbeats table should exist after migrations run")
	require.Equal(t, 0, count)
}

func TestOpenPersistsAcrossRestarts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")

	db1, err := Open(dbPath)
	require.NoError(t, err)

	_, err = db1.Exec("INSERT INTO heartbeats DEFAULT VALUES")
	require.NoError(t, err)
	require.NoError(t, db1.Close())

	// Simulate a process restart: reopen against the same path.
	db2, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db2.Close()) })

	var count int
	err = db2.QueryRow("SELECT COUNT(*) FROM heartbeats").Scan(&count)
	require.NoError(t, err)
	require.Equal(t, 1, count, "row written before restart should survive reopening the same db file")
}

func TestOpenIsIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")

	db1, err := Open(dbPath)
	require.NoError(t, err)
	require.NoError(t, db1.Close())

	db2, err := Open(dbPath)
	require.NoError(t, err, "reopening and re-migrating an already-migrated db should not error")
	t.Cleanup(func() { require.NoError(t, db2.Close()) })
}
