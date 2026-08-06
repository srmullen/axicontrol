package api

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/srmullen/axicontrol/internal/svg"
)

// maxUploadBytes bounds a single upload; SVGs are text-based vector data, so
// this comfortably covers legitimate designs while bounding request size.
const maxUploadBytes = 10 << 20 // 10MiB

type uploadView struct {
	ID        int64
	Filename  string
	SizeBytes int64
	CreatedAt string
}

type uploadsSectionView struct {
	Uploads []uploadView
	Error   string
}

func (s *Server) loadUploads(r *http.Request) ([]uploadView, error) {
	rows, err := s.db.QueryContext(r.Context(),
		"SELECT id, filename, size_bytes, created_at FROM files ORDER BY created_at DESC, id DESC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	uploads := []uploadView{}
	for rows.Next() {
		var v uploadView
		if err := rows.Scan(&v.ID, &v.Filename, &v.SizeBytes, &v.CreatedAt); err != nil {
			return nil, err
		}
		uploads = append(uploads, v)
	}
	return uploads, rows.Err()
}

func (s *Server) handleListUploads(w http.ResponseWriter, r *http.Request) {
	uploads, err := s.loadUploads(r)
	if err != nil {
		s.logger.Error("list uploads failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.renderPage(w, "uploads_content", uploadsSectionView{Uploads: uploads})
}

// rerenderUploadsSection re-loads the upload list and renders it plus the
// upload form (with errMsg, if any) as the #uploads-section fragment.
func (s *Server) rerenderUploadsSection(w http.ResponseWriter, r *http.Request, status int, errMsg string) {
	uploads, err := s.loadUploads(r)
	if err != nil {
		s.logger.Error("list uploads failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderFragment(w, status, "uploads_section", uploadsSectionView{Uploads: uploads, Error: errMsg})
}

func (s *Server) handleCreateUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		s.rerenderUploadsSection(w, r, http.StatusOK, "upload too large or malformed")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		s.rerenderUploadsSection(w, r, http.StatusOK, "a file is required")
		return
	}
	defer func() { _ = file.Close() }()

	raw, err := io.ReadAll(file)
	if err != nil {
		s.logger.Error("read upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	sanitized, err := svg.Sanitize(raw)
	if err != nil {
		s.rerenderUploadsSection(w, r, http.StatusOK, "file must be a valid SVG")
		return
	}

	key := uuid.NewString() + ".svg"
	if err := s.files.Put(key, bytes.NewReader(sanitized)); err != nil {
		s.logger.Error("store upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = s.db.ExecContext(r.Context(),
		"INSERT INTO files (filename, storage_key, size_bytes) VALUES (?, ?, ?)",
		header.Filename, key, len(sanitized))
	if err != nil {
		s.logger.Error("insert file record failed", "error", err)
		if delErr := s.files.Delete(key); delErr != nil {
			s.logger.Error("cleanup orphaned upload failed", "error", delErr)
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.rerenderUploadsSection(w, r, http.StatusOK, "")
}

func uploadIDFromPath(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) handleShowUpload(w http.ResponseWriter, r *http.Request) {
	id, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	v, ok := s.loadUploadOrNotFound(w, r, id)
	if !ok {
		return
	}

	s.renderFragment(w, http.StatusOK, "upload_preview", v)
}

// loadUploadOrNotFound loads an upload by id, writing the appropriate error
// response itself (404 or 500) and returning ok=false when there's nothing
// for the caller to render.
func (s *Server) loadUploadOrNotFound(w http.ResponseWriter, r *http.Request, id int64) (uploadView, bool) {
	var v uploadView
	row := s.db.QueryRowContext(r.Context(), "SELECT id, filename FROM files WHERE id = ?", id)
	err := row.Scan(&v.ID, &v.Filename)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "file not found")
		return uploadView{}, false
	}
	if err != nil {
		s.logger.Error("load file record failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return uploadView{}, false
	}
	return v, true
}

// loadStorageKeyOrNotFound loads an upload's storage key by id, writing the
// appropriate error response itself (404 or 500) and returning ok=false when
// there's nothing for the caller to act on.
func (s *Server) loadStorageKeyOrNotFound(w http.ResponseWriter, r *http.Request, id int64) (string, bool) {
	var key string
	row := s.db.QueryRowContext(r.Context(), "SELECT storage_key FROM files WHERE id = ?", id)
	err := row.Scan(&key)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "file not found")
		return "", false
	}
	if err != nil {
		s.logger.Error("load file record failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	return key, true
}

func (s *Server) handleUploadContent(w http.ResponseWriter, r *http.Request) {
	id, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	key, ok := s.loadStorageKeyOrNotFound(w, r, id)
	if !ok {
		return
	}

	rc, err := s.files.Get(key)
	if err != nil {
		s.logger.Error("read stored file failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = rc.Close() }()

	// Served only via <img> (never inline, <object>, or <iframe>) so this
	// content runs in the browser's strictest built-in security context.
	w.Header().Set("Content-Type", "image/svg+xml")
	_, _ = io.Copy(w, rc)
}

func (s *Server) handleDeleteUpload(w http.ResponseWriter, r *http.Request) {
	id, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	key, ok := s.loadStorageKeyOrNotFound(w, r, id)
	if !ok {
		return
	}

	if _, err := s.db.ExecContext(r.Context(), "DELETE FROM files WHERE id = ?", id); err != nil {
		s.logger.Error("delete file record failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := s.files.Delete(key); err != nil {
		// The DB row (source of truth for the library listing) is already
		// gone; log the orphaned blob rather than failing the request.
		s.logger.Error("delete stored file failed", "error", err)
	}

	w.WriteHeader(http.StatusOK)
}
