package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSysinfoSuccess(t *testing.T) {
	s := NewServer(nil, "", testLogger())
	var gotArgs []string
	s.runAxicli = func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("model: AxiDraw V3\n"), nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sysinfo", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, []string{"sysinfo"}, gotArgs, "no device path configured, should not pass --port")

	var body struct {
		Output string `json:"output"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Equal(t, "model: AxiDraw V3\n", body.Output)
}

func TestHandleSysinfoPassesDevicePath(t *testing.T) {
	s := NewServer(nil, "/dev/axidraw", testLogger())
	var gotArgs []string
	s.runAxicli = func(args ...string) ([]byte, error) {
		gotArgs = args
		return []byte("ok"), nil
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sysinfo", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, []string{"sysinfo", "--port", "/dev/axidraw"}, gotArgs)
}

func TestHandleSysinfoCommandFailure(t *testing.T) {
	s := NewServer(nil, "", testLogger())
	s.runAxicli = func(args ...string) ([]byte, error) {
		return []byte("device not found"), errors.New("exit status 1")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sysinfo", nil)
	s.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadGateway, rr.Code)

	var body struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	assert.Contains(t, body.Error, "exit status 1")
}
