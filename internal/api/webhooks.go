package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
)

// webhookView is a registered outbound webhook destination (ADR-0006).
type webhookView struct {
	ID  int64
	URL string
}

type webhooksSectionView struct {
	Webhooks []webhookView
	Error    string
}

func (s *Server) loadWebhooks(ctx context.Context) ([]webhookView, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, url FROM webhooks ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	webhooks := []webhookView{}
	for rows.Next() {
		var v webhookView
		if err := rows.Scan(&v.ID, &v.URL); err != nil {
			return nil, err
		}
		webhooks = append(webhooks, v)
	}
	return webhooks, rows.Err()
}

// parseWebhookURL validates a user-supplied destination: axicontrol only
// ever POSTs to it directly (ADR-0006), so it must be an absolute http(s)
// URL — nothing else is a meaningful target for that.
func parseWebhookURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("url is required")
	}
	u, err := url.ParseRequestURI(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", errors.New("url must be an absolute http(s) URL")
	}
	return raw, nil
}

func (s *Server) handleListWebhooks(w http.ResponseWriter, r *http.Request) {
	webhooks, err := s.loadWebhooks(r.Context())
	if err != nil {
		s.logger.Error("list webhooks failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.renderPage(w, "webhooks_content", webhooksSectionView{Webhooks: webhooks})
}

func (s *Server) handleCreateWebhook(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	validURL, err := parseWebhookURL(r.FormValue("url"))
	if err != nil {
		s.rerenderWebhooksSection(w, r, http.StatusOK, err.Error())
		return
	}

	_, err = s.db.ExecContext(r.Context(), "INSERT INTO webhooks (url) VALUES (?)", validURL)
	if isUniqueConstraintErr(err) {
		s.rerenderWebhooksSection(w, r, http.StatusOK, "that URL is already registered")
		return
	}
	if err != nil {
		s.logger.Error("create webhook failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.rerenderWebhooksSection(w, r, http.StatusOK, "")
}

// rerenderWebhooksSection re-loads the full webhook list and renders it plus
// the register form (with formErr, if any) as the #webhooks-section
// fragment.
func (s *Server) rerenderWebhooksSection(w http.ResponseWriter, r *http.Request, status int, formErr string) {
	webhooks, err := s.loadWebhooks(r.Context())
	if err != nil {
		s.logger.Error("list webhooks failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderFragment(w, status, "webhooks_section", webhooksSectionView{Webhooks: webhooks, Error: formErr})
}

func (s *Server) handleDeleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid webhook id")
		return
	}

	res, err := s.db.ExecContext(r.Context(), "DELETE FROM webhooks WHERE id = ?", id)
	if err != nil {
		s.logger.Error("delete webhook failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}

	w.WriteHeader(http.StatusOK)
}
