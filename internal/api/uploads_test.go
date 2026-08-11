package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/srmullen/axicontrol/internal/filestore"
	"github.com/srmullen/axicontrol/internal/store"
	"github.com/srmullen/axicontrol/internal/svg"
)

const validSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><rect width="10" height="10"/></svg>`

const maliciousSVG = `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">
  <script>alert('xss')</script>
  <rect width="10" height="10" onload="alert('xss')"/>
</svg>`

var uploadRowIDPattern = regexp.MustCompile(`id="upload-(\d+)"`)

func firstUploadID(t *testing.T, body string) int64 {
	t.Helper()
	match := uploadRowIDPattern.FindStringSubmatch(body)
	require.NotNil(t, match, "expected an upload row id in body: %s", body)
	id, err := strconv.ParseInt(match[1], 10, 64)
	require.NoError(t, err)
	return id
}

func doMultipartUpload(t *testing.T, s *Server, filename string, content []byte) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, mw.Close())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/uploads", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	s.ServeHTTP(rr, req)
	return rr
}

func TestIndexRedirectsToUploads(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusFound, rr.Code)
	require.Equal(t, "/uploads", rr.Header().Get("Location"))
}

func TestUploadsSectionHasDropzoneWrappingForm(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	body := rr.Body.String()

	dropzoneStart := strings.Index(body, `id="upload-dropzone"`)
	formStart := strings.Index(body, `hx-post="/uploads"`)
	fileInput := strings.Index(body, `type="file"`)
	dropzoneEnd := strings.LastIndex(body, "</div>")

	require.NotEqual(t, -1, dropzoneStart, "expected an upload-dropzone element")
	require.Greater(t, formStart, dropzoneStart, "expected the upload form after the dropzone opening tag")
	require.Greater(t, fileInput, formStart, "expected the file input inside the form")
	require.Greater(t, dropzoneEnd, fileInput, "expected the file input inside the dropzone")
}

func TestUploadsListEmpty(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestUploadThenList(t *testing.T) {
	s := newTestServer(t)

	rr := doMultipartUpload(t, s, "drawing.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "drawing.svg")

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "drawing.svg")
}

func TestUploadRejectsNonSVG(t *testing.T) {
	s := newTestServer(t)

	rr := doMultipartUpload(t, s, "notes.txt", []byte("just some text, not an SVG at all"))
	// htmx only swaps a fragment into the DOM on a 2xx response by default,
	// so the inline error must ship as 200 for the user to see it.
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "SVG")

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	s.ServeHTTP(rr, req)
	require.NotContains(t, rr.Body.String(), "notes.txt")
}

func TestUploadContentIsSanitized(t *testing.T) {
	s := newTestServer(t)

	rr := doMultipartUpload(t, s, "evil.svg", []byte(maliciousSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	id := firstUploadID(t, rr.Body.String())

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(id)+"/content", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "image/svg+xml", rr.Header().Get("Content-Type"))
	require.NotContains(t, rr.Body.String(), "<script")
	require.NotContains(t, rr.Body.String(), "onload")
	require.Contains(t, rr.Body.String(), `width="10"`)
}

func TestUploadLayerContentServesOnlyTheChosenLayer(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s) // layeredSVG: "1 black", "2 red"

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/layers/2/content", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "image/svg+xml", rr.Header().Get("Content-Type"))
	layers, err := svg.DiscoverLayers(rr.Body.Bytes())
	require.NoError(t, err)
	require.Equal(t, []int{2}, svg.Numbers(layers), "only the requested layer's group should remain")
}

func TestUploadLayerContentUnknownLayerReturnsNotFound(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/layers/99/content", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestUploadLayerContentMissingUploadReturnsNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/999/layers/1/content", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestUploadLayerContentInvalidLayerNumberReturnsBadRequest(t *testing.T) {
	s := newTestServer(t)
	fileID, _ := seedLayeredFileAndPreset(t, s)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(fileID)+"/layers/not-a-number/content", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestUploadRowLinksToPrintPageNotInlinePreview(t *testing.T) {
	s := newTestServer(t)

	rr := doMultipartUpload(t, s, "drawing.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	id := firstUploadID(t, rr.Body.String())

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	body := rr.Body.String()
	require.Contains(t, body, `href="/uploads/`+itoa(id)+`/print"`)
	require.NotContains(t, body, `hx-get="/uploads/`+itoa(id)+`"`, "the inline Preview button should be gone")
	require.NotContains(t, body, `id="upload-preview"`, "the inline preview panel should be gone")
}

func TestUploadDelete(t *testing.T) {
	s := newTestServer(t)

	rr := doMultipartUpload(t, s, "drawing.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	id := firstUploadID(t, rr.Body.String())

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/uploads/"+itoa(id), nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(id)+"/content", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/uploads", nil)
	s.ServeHTTP(rr, req)
	require.NotContains(t, rr.Body.String(), "drawing.svg")
}

func TestUploadDeleteMissingReturnsNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/uploads/999", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestUploadsPersistAcrossReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "axicontrol.sqlite")
	filesRoot := t.TempDir()

	db, err := store.Open(dbPath)
	require.NoError(t, err)
	fs1, err := filestore.NewPVStore(filesRoot)
	require.NoError(t, err)
	s := NewServer(db, fs1, "", testLogger())

	require.Equal(t, http.StatusOK, doMultipartUpload(t, s, "drawing.svg", []byte(validSVG)).Code)
	require.NoError(t, db.Close())

	db2, err := store.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db2.Close()) })
	fs2, err := filestore.NewPVStore(filesRoot)
	require.NoError(t, err)
	s2 := NewServer(db2, fs2, "", testLogger())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads", nil)
	s2.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "drawing.svg")

	id := firstUploadID(t, rr.Body.String())
	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(id)+"/content", nil)
	s2.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "<rect")
}
