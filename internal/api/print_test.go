package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrintPageShowsWholeDocumentPreview(t *testing.T) {
	s := newTestServer(t)

	rr := doMultipartUpload(t, s, "drawing.svg", []byte(validSVG))
	require.Equal(t, http.StatusOK, rr.Code)
	id := firstUploadID(t, rr.Body.String())

	rr = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/"+itoa(id)+"/print", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	body := rr.Body.String()
	require.Contains(t, body, "drawing.svg")
	require.Contains(t, body, "<img")
	require.Contains(t, body, "/uploads/"+itoa(id)+"/content")
	require.NotContains(t, body, "<object")
	require.NotContains(t, body, "<iframe")
}

func TestPrintPageMissingUploadReturnsNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/999/print", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}

func TestPrintPageInvalidIDReturnsBadRequest(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/uploads/not-a-number/print", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusBadRequest, rr.Code)
}
