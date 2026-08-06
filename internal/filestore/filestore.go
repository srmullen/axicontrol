// Package filestore abstracts durable blob storage (put/get/delete) behind
// an interface, so callers don't couple to a particular storage technology.
// The initial adapter is backed by a PersistentVolume-mounted directory.
package filestore

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FileStore puts, gets, and deletes byte blobs by key.
type FileStore interface {
	Put(key string, r io.Reader) error
	Get(key string) (io.ReadCloser, error)
	Delete(key string) error

	// LocalPath returns a filesystem path holding key's contents, for
	// callers (like axicli, spawned as a subprocess) that need a real path
	// rather than a stream. The returned cleanup func must be called once
	// the caller is done with the path; adapters without direct local
	// filesystem access would materialize a temp file here and delete it on
	// cleanup, but PVStore's own directory already is that filesystem, so
	// its cleanup is a no-op.
	LocalPath(key string) (path string, cleanup func(), err error)
}

// PVStore is a FileStore backed by a directory on a mounted filesystem
// (a Kubernetes PersistentVolume in production). Each key maps to exactly
// one file directly under root.
type PVStore struct {
	root string
}

// NewPVStore creates (if necessary) root and returns a PVStore rooted there.
func NewPVStore(root string) (*PVStore, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create filestore root: %w", err)
	}
	return &PVStore{root: root}, nil
}

func (s *PVStore) Put(key string, r io.Reader) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write file: %w", err)
	}
	return nil
}

func (s *PVStore) Get(key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	return f, nil
}

func (s *PVStore) Delete(key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove file: %w", err)
	}
	return nil
}

func (s *PVStore) LocalPath(key string) (string, func(), error) {
	path, err := s.path(key)
	if err != nil {
		return "", nil, err
	}

	if _, err := os.Stat(path); err != nil {
		return "", nil, fmt.Errorf("stat file: %w", err)
	}

	return path, func() {}, nil
}

// path joins key onto root, rejecting keys that could escape root (no
// separators, no "..") since keys are expected to be server-generated,
// single-segment file names.
func (s *PVStore) path(key string) (string, error) {
	if key == "" || key != filepath.Base(key) || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid filestore key %q", key)
	}
	return filepath.Join(s.root, key), nil
}
