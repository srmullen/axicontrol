package filestore

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPVStoreCreatesRootDir(t *testing.T) {
	root := filepath.Join(t.TempDir(), "nested", "files")
	_, err := NewPVStore(root)
	require.NoError(t, err)

	info, err := os.Stat(root)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestPVStorePutThenGet(t *testing.T) {
	s, err := NewPVStore(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, s.Put("a.svg", strings.NewReader("hello")))

	rc, err := s.Get("a.svg")
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}

func TestPVStoreGetMissingKeyReturnsNotExist(t *testing.T) {
	s, err := NewPVStore(t.TempDir())
	require.NoError(t, err)

	_, err = s.Get("missing.svg")
	require.Error(t, err)
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestPVStoreDeleteThenGetIsNotExist(t *testing.T) {
	s, err := NewPVStore(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, s.Put("a.svg", strings.NewReader("hello")))
	require.NoError(t, s.Delete("a.svg"))

	_, err = s.Get("a.svg")
	require.True(t, errors.Is(err, os.ErrNotExist))
}

func TestPVStoreDeleteMissingKeyIsNoop(t *testing.T) {
	s, err := NewPVStore(t.TempDir())
	require.NoError(t, err)

	require.NoError(t, s.Delete("never-existed.svg"))
}

func TestPVStorePersistsAcrossReopen(t *testing.T) {
	root := t.TempDir()
	s1, err := NewPVStore(root)
	require.NoError(t, err)
	require.NoError(t, s1.Put("a.svg", strings.NewReader("hello")))

	s2, err := NewPVStore(root)
	require.NoError(t, err)
	rc, err := s2.Get("a.svg")
	require.NoError(t, err)
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.Equal(t, "hello", string(data))
}

func TestPVStoreRejectsPathTraversalKey(t *testing.T) {
	s, err := NewPVStore(t.TempDir())
	require.NoError(t, err)

	require.Error(t, s.Put("../escape.svg", strings.NewReader("hello")))
	require.Error(t, s.Put("nested/escape.svg", strings.NewReader("hello")))
	_, err = s.Get("../escape.svg")
	require.Error(t, err)
}
