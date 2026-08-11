package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// webhookReceiver is a tiny httptest server that records every JSON payload
// POSTed to it, for asserting on what a webhook delivery actually contained.
type webhookReceiver struct {
	server *httptest.Server

	mu       sync.Mutex
	payloads []jobEvent
}

func newWebhookReceiver(t *testing.T) *webhookReceiver {
	t.Helper()
	wr := &webhookReceiver{}
	wr.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var evt jobEvent
		_ = json.NewDecoder(r.Body).Decode(&evt)
		wr.mu.Lock()
		wr.payloads = append(wr.payloads, evt)
		wr.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(wr.server.Close)
	return wr
}

func (wr *webhookReceiver) count() int {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	return len(wr.payloads)
}

func (wr *webhookReceiver) last() jobEvent {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	return wr.payloads[len(wr.payloads)-1]
}

func TestWebhookFiresOnJobComplete(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	wr := newWebhookReceiver(t)
	registerWebhook(t, s, wr.server.URL)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("plot complete"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return wr.count() > 0
	}, 2*time.Second, 10*time.Millisecond, "webhook was never called")

	evt := wr.last()
	require.Equal(t, "job.complete", evt.Event)
	require.Equal(t, jobID, evt.JobID)
	require.Equal(t, "complete", evt.Status)
}

func TestWebhookFiresOnPassFailed(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	wr := newWebhookReceiver(t)
	registerWebhook(t, s, wr.server.URL)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("device not found"), errors.New("exit status 1")
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return wr.count() > 0
	}, 2*time.Second, 10*time.Millisecond, "webhook was never called")

	evt := wr.last()
	require.Equal(t, "pass.failed", evt.Event)
	require.Equal(t, jobID, evt.JobID)
	require.Equal(t, "failed", evt.Status)
	require.Contains(t, evt.Output, "exit status 1")
}

func TestWebhookFiresOnJobAwaitingNextPass(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedLayeredFileAndPreset(t, s)
	wr := newWebhookReceiver(t)
	registerWebhook(t, s, wr.server.URL)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("layer plotted"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
		"mode":      {"layers"},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	require.Eventually(t, func() bool {
		return wr.count() > 0
	}, 2*time.Second, 10*time.Millisecond, "webhook was never called")

	evt := wr.last()
	require.Equal(t, "job.awaiting-next-pass", evt.Event)
	require.Equal(t, jobID, evt.JobID)
	require.Equal(t, "awaiting-next-pass", evt.Status)

	// The Job isn't done yet, so it must not also fire job.complete.
	require.Equal(t, 1, wr.count())
}

func TestWebhookDoesNotFireOnPauseResumeOrCancel(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	wr := newWebhookReceiver(t)
	registerWebhook(t, s, wr.server.URL)

	fakeRun, argsCh := interruptibleAxicli()
	s.runAxicli = fakeRun

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	select {
	case <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	rr = postJob(t, s, jobID, "pause")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "paused")
	}, 2*time.Second, 10*time.Millisecond)
	require.Equal(t, 0, wr.count(), "pause must never fire a webhook")

	rr = postJob(t, s, jobID, "resume")
	require.Equal(t, http.StatusOK, rr.Code)
	select {
	case <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked for resume")
	}
	require.Equal(t, 0, wr.count(), "resume must never fire a webhook")

	rr = postJob(t, s, jobID, "cancel")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "cancelled")
	}, 2*time.Second, 10*time.Millisecond)

	require.Equal(t, 0, wr.count(), "user-initiated pause/resume/cancel must never fire a webhook")
}

func TestWebhookFiresToMultipleRegisteredURLs(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)
	wr1 := newWebhookReceiver(t)
	wr2 := newWebhookReceiver(t)
	registerWebhook(t, s, wr1.server.URL)
	registerWebhook(t, s, wr2.server.URL)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)

	require.Eventually(t, func() bool {
		return wr1.count() > 0 && wr2.count() > 0
	}, 2*time.Second, 10*time.Millisecond, "both registered webhooks must receive the event")

	require.Equal(t, "job.complete", wr1.last().Event)
	require.Equal(t, "job.complete", wr2.last().Event)
}

func TestSSEStreamReceivesJobUpdateOnComplete(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	httpServer := httptest.NewServer(s)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))

	lines := make(chan string, 256)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	require.Eventually(t, func() bool {
		return s.subscriberCount() > 0
	}, 2*time.Second, 5*time.Millisecond, "SSE client never subscribed")

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	var sawEventName, sawOOBRow bool
	deadline := time.After(3 * time.Second)
	for !sawOOBRow {
		select {
		case line := <-lines:
			if line == "event: job-update" {
				sawEventName = true
			}
			if strings.Contains(line, `id="job-`+strconv.FormatInt(jobID, 10)+`"`) && strings.Contains(line, "hx-swap-oob") {
				sawOOBRow = true
			}
		case <-deadline:
			t.Fatal("did not receive the expected job-update SSE event in time")
		}
	}
	require.True(t, sawEventName, "expected a named job-update SSE event")
}

// TestSSEStreamJobUpdateIncludesPrintPageStatusFragment verifies the same
// job-update event a connected /jobs page consumes also carries an
// OOB-swap fragment for the print page's live status card (ADR-0017,
// print_job_status) — one SSE broadcast serving both a /jobs table row and
// a print page's status card, distinguished only by which id a given
// client's own DOM happens to contain.
func TestSSEStreamJobUpdateIncludesPrintPageStatusFragment(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	s.runAxicli = func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("ok"), nil
	}

	httpServer := httptest.NewServer(s)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	lines := make(chan string, 256)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	require.Eventually(t, func() bool {
		return s.subscriberCount() > 0
	}, 2*time.Second, 5*time.Millisecond, "SSE client never subscribed")

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	var sawPrintOOB bool
	deadline := time.After(3 * time.Second)
	for !sawPrintOOB {
		select {
		case line := <-lines:
			if strings.Contains(line, `id="print-job-`+strconv.FormatInt(jobID, 10)+`"`) && strings.Contains(line, "hx-swap-oob") {
				sawPrintOOB = true
			}
		case <-deadline:
			t.Fatal("did not receive the print page's OOB status fragment in time")
		}
	}
}

func TestSSEStreamDoesNotReceiveUpdateOnPause(t *testing.T) {
	s := newTestServer(t)
	fileID, presetID := seedFileAndPreset(t, s)

	fakeRun, argsCh := interruptibleAxicli()
	s.runAxicli = fakeRun

	httpServer := httptest.NewServer(s)
	defer httpServer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, httpServer.URL+"/events", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	lines := make(chan string, 256)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()

	require.Eventually(t, func() bool {
		return s.subscriberCount() > 0
	}, 2*time.Second, 5*time.Millisecond, "SSE client never subscribed")

	rr := submitJob(t, s, fileID, url.Values{
		"preset_id": {strconv.FormatInt(presetID, 10)},
	})
	require.Equal(t, http.StatusOK, rr.Code)
	jobID := firstPrintJobID(t, rr.Body.String())

	select {
	case <-argsCh:
	case <-time.After(2 * time.Second):
		t.Fatal("axicli was not invoked")
	}

	rr = postJob(t, s, jobID, "pause")
	require.Equal(t, http.StatusOK, rr.Code)
	require.Eventually(t, func() bool {
		return strings.Contains(jobRow(t, s, jobID), "paused")
	}, 2*time.Second, 10*time.Millisecond)

	select {
	case line := <-lines:
		t.Fatalf("pause must not produce an SSE event, got: %s", line)
	case <-time.After(200 * time.Millisecond):
	}
}
