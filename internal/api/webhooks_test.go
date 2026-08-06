package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

var webhookRowIDPattern = regexp.MustCompile(`id="webhook-(\d+)"`)

func firstWebhookID(t *testing.T, body string) int64 {
	t.Helper()
	match := webhookRowIDPattern.FindStringSubmatch(body)
	require.NotNil(t, match, "expected a webhook row id in body: %s", body)
	id, err := strconv.ParseInt(match[1], 10, 64)
	require.NoError(t, err)
	return id
}

// registerWebhook registers targetURL through the app's own /webhooks
// endpoint, returning its assigned id.
func registerWebhook(t *testing.T, s *Server, targetURL string) int64 {
	t.Helper()
	rr := doForm(t, s, http.MethodPost, "/webhooks", url.Values{"url": {targetURL}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), targetURL)
	return firstWebhookID(t, rr.Body.String())
}

func TestWebhooksListEmpty(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
}

func TestWebhookRegisterAndList(t *testing.T) {
	s := newTestServer(t)

	registerWebhook(t, s, "https://example.com/hook")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "https://example.com/hook")
}

func TestWebhookRegisterRejectsBlankURL(t *testing.T) {
	s := newTestServer(t)

	rr := doForm(t, s, http.MethodPost, "/webhooks", url.Values{"url": {""}})
	// htmx only swaps a fragment into the DOM on a 2xx response by default,
	// so the inline error must ship as 200 to actually render.
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "required")
}

func TestWebhookRegisterRejectsNonHTTPURL(t *testing.T) {
	s := newTestServer(t)

	rr := doForm(t, s, http.MethodPost, "/webhooks", url.Values{"url": {"ftp://example.com/hook"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "http(s)")
}

func TestWebhookRegisterRejectsMalformedURL(t *testing.T) {
	s := newTestServer(t)

	rr := doForm(t, s, http.MethodPost, "/webhooks", url.Values{"url": {"not a url"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "http(s)")
}

func TestWebhookRegisterRejectsDuplicate(t *testing.T) {
	s := newTestServer(t)
	registerWebhook(t, s, "https://example.com/hook")

	rr := doForm(t, s, http.MethodPost, "/webhooks", url.Values{"url": {"https://example.com/hook"}})
	require.Equal(t, http.StatusOK, rr.Code)
	require.Contains(t, rr.Body.String(), "already registered")
}

func TestWebhookDelete(t *testing.T) {
	s := newTestServer(t)
	id := registerWebhook(t, s, "https://example.com/hook")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/webhooks/"+strconv.FormatInt(id, 10), nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/webhooks", nil)
	s.ServeHTTP(rr, req)
	require.NotContains(t, rr.Body.String(), `id="webhook-`+strconv.FormatInt(id, 10)+`"`)
}

func TestWebhookDeleteUnknownIDNotFound(t *testing.T) {
	s := newTestServer(t)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/webhooks/999", nil)
	s.ServeHTTP(rr, req)
	require.Equal(t, http.StatusNotFound, rr.Code)
}
