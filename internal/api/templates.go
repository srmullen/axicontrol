package api

import (
	"bytes"
	"embed"
	"html/template"
	"io/fs"
	"net/http"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

func parseTemplates() *template.Template {
	return template.Must(template.ParseFS(templatesFS, "templates/*.html"))
}

func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServerFS(sub)
}

// renderPage executes contentTemplate and wraps the result in the shared
// layout, for full browser navigations (GET requests to a page's own URL).
func (s *Server) renderPage(w http.ResponseWriter, contentTemplate string, data any) {
	var content bytes.Buffer
	if err := s.templates.ExecuteTemplate(&content, contentTemplate, data); err != nil {
		s.logger.Error("render content template failed", "template", contentTemplate, "error", err)
		writeError(w, http.StatusInternalServerError, "render failed")
		return
	}

	var page bytes.Buffer
	pageData := struct{ Content template.HTML }{template.HTML(content.String())} //nolint:gosec // content is server-rendered from our own templates, not user input
	if err := s.templates.ExecuteTemplate(&page, "layout", pageData); err != nil {
		s.logger.Error("render layout failed", "error", err)
		writeError(w, http.StatusInternalServerError, "render failed")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = page.WriteTo(w)
}

// renderFragment executes a single named template and writes it directly,
// for htmx requests that swap one element rather than navigating.
func (s *Server) renderFragment(w http.ResponseWriter, status int, fragmentTemplate string, data any) {
	var buf bytes.Buffer
	if err := s.templates.ExecuteTemplate(&buf, fragmentTemplate, data); err != nil {
		s.logger.Error("render fragment template failed", "template", fragmentTemplate, "error", err)
		writeError(w, http.StatusInternalServerError, "render failed")
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
