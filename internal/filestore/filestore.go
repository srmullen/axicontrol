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
	// its cleanup is a no-op. Returns an error if key doesn't already exist.
	LocalPath(key string) (path string, cleanup func(), err error)

	// LocalWritePath returns a filesystem path a caller (like axicli, via
	// its -o flag) can write key's contents to directly, unlike LocalPath,
	// key need not already exist — first-time output creates it, and a
	// later call against the same key (e.g. axicli overwriting a
	// checkpoint on each pause) resolves to the same path in place. The
	// returned cleanup func follows the same contract as LocalPath's.
	LocalWritePath(key string) (path string, cleanup func(), err error)
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
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

func (s *PVStore) LocalWritePath(key string) (string, func(), error) {
	path, err := s.path(key)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", nil, fmt.Errorf("create parent dir: %w", err)
	}
	return path, func() {}, nil
}

// path joins key onto root, rejecting keys that could escape root or use
// unexpected nesting. Keys are always server-generated, never user input:
// either a single-segment file name (uploads) or one under the
// "checkpoints/" namespace (Pass checkpoints — ADR-0008), which is itself a
// single segment once that prefix is stripped. Anything containing ".." or
// nested any other way is rejected.
func (s *PVStore) path(key string) (string, error) {
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid filestore key %q", key)
	}
	base := key
	if rest, ok := strings.CutPrefix(key, "checkpoints/"); ok {
		base = rest
	}
	if base == "" || base != filepath.Base(base) {
		return "", fmt.Errorf("invalid filestore key %q", key)
	}
	return filepath.Join(s.root, key), nil
}
