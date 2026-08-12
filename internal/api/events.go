package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// webhookTimeout bounds a single webhook delivery attempt so an unreachable
// or slow destination can't accumulate goroutines across repeated Job
// transitions.
const webhookTimeout = 10 * time.Second

// jobEvent is fired on the three Job/Pass transitions ADR-0006 identified as
// needing attention while unattended — a Job reaching complete, a Pass
// reaching failed, or a Job reaching awaiting-next-pass. User-initiated
// transitions (pause/resume/cancel) never produce one, since the user
// already knows they happened.
type jobEvent struct {
	Event     string `json:"event"` // "job.complete", "pass.failed", or "job.awaiting-next-pass"
	JobID     int64  `json:"job_id"`
	PassID    int64  `json:"pass_id"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	Timestamp string `json:"timestamp"`
}

func newJobEvent(event string, jobID, passID int64, status, output string) jobEvent {
	return jobEvent{
		Event:     event,
		JobID:     jobID,
		PassID:    passID,
		Status:    status,
		Output:    output,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
}

// subscribe registers a new SSE client, returning a channel it should read
// events from and an unsubscribe func the caller must run once the client
// disconnects. The channel is buffered so a slow client can't block notify's
// caller (executePass's own goroutine, which is what's actually driving the
// plot) — a client that falls far enough behind just misses events rather
// than stalling plotting.
func (s *Server) subscribe() (chan jobEvent, func()) {
	ch := make(chan jobEvent, 16)
	s.subMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subMu.Unlock()

	return ch, func() {
		s.subMu.Lock()
		delete(s.subscribers, ch)
		s.subMu.Unlock()
	}
}

// subscriberCount reports how many SSE clients are currently connected.
func (s *Server) subscriberCount() int {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	return len(s.subscribers)
}

func (s *Server) publish(evt jobEvent) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- evt:
		default:
			// Slow subscriber: drop rather than block the publisher.
		}
	}
}

// notify fires evt to every connected SSE client and every registered
// webhook URL — the two channels ADR-0006 decided on for surfacing a Job/
// Pass transition that needs attention while unattended.
func (s *Server) notify(ctx context.Context, evt jobEvent) {
	s.publish(evt)
	s.fireWebhooks(ctx, evt)
}

// fireWebhooks POSTs evt as JSON to every registered webhook URL, each in
// its own goroutine so a slow or unreachable destination can't delay
// plotting or block delivery to other destinations (ADR-0006; stdlib
// net/http client per ADR-0011).
func (s *Server) fireWebhooks(ctx context.Context, evt jobEvent) {
	webhooks, err := s.loadWebhooks(ctx)
	if err != nil {
		s.logger.Error("load webhooks for notify failed", "error", err)
		return
	}
	if len(webhooks) == 0 {
		return
	}

	body, err := json.Marshal(evt)
	if err != nil {
		s.logger.Error("marshal webhook payload failed", "error", err)
		return
	}

	for _, wh := range webhooks {
		go s.postWebhook(wh.URL, body)
	}
}

// postWebhook delivers body to url, independent of whatever context
// triggered the notification — the caller (executePass) has already moved
// on by the time delivery completes. s.httpClient's own Timeout (set at
// construction) bounds the whole attempt.
func (s *Server) postWebhook(url string, body []byte) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		s.logger.Error("build webhook request failed", "url", url, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.logger.Error("webhook delivery failed", "url", url, "error", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		s.logger.Error("webhook delivery rejected", "url", url, "status", resp.StatusCode)
	}
}

// handleEvents streams Job/Pass state changes to a connected client via
// Server-Sent Events (ADR-0006), sourced from the same jobEvents notify
// fires at the webhook. The htmx SSE extension on the Jobs page (ADR-0012)
// consumes it to keep the job list live without polling or a page reload.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	ch, cancel := s.subscribe()
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			s.writeJobUpdateEvent(w, r.Context(), evt)
			flusher.Flush()
		}
	}
}

// writeJobUpdateEvent writes evt's Job as a named "job-update" SSE message,
// its data both the Job's freshly rendered /jobs row fragment and its print
// page status fragment (see print_job_status), each marked hx-swap-oob with
// its own distinct id so the htmx SSE extension swaps whichever one a given
// connected client's DOM actually contains — the /jobs table row, the print
// page's status card, or (having neither) nothing — without a separate
// round trip back to the server.
func (s *Server) writeJobUpdateEvent(w io.Writer, ctx context.Context, evt jobEvent) {
	row, err := s.loadJobRow(ctx, evt.JobID)
	if err != nil {
		s.logger.Error("load job row for sse failed", "job_id", evt.JobID, "error", err)
		return
	}
	row.OOB = true

	statusView, err := s.buildPrintJobStatusView(ctx, row)
	if err != nil {
		s.logger.Error("build print job status for sse failed", "job_id", evt.JobID, "error", err)
		return
	}

	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, "job_row", row); err != nil {
		s.logger.Error("render sse row failed", "error", err)
		return
	}
	if err := s.templates.ExecuteTemplate(&buf, "print_job_status", statusView); err != nil {
		s.logger.Error("render sse print status failed", "error", err)
		return
	}

	var frame strings.Builder
	frame.WriteString("event: job-update\n")
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		frame.WriteString("data: ")
		frame.WriteString(line)
		frame.WriteString("\n")
	}
	frame.WriteString("\n")

	if _, err := io.WriteString(w, frame.String()); err != nil {
		s.logger.Error("write sse event failed", "error", err)
	}
}
