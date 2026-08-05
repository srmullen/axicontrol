// Package api implements axicontrol's HTTP surface.
package api

import (
	"database/sql"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
)

// Server is axicontrol's HTTP handler, wired to the device (via axicli) and
// the embedded SQLite datastore.
type Server struct {
	mux        *http.ServeMux
	db         *sql.DB
	devicePath string
	logger     *slog.Logger
	runAxicli  func(args ...string) ([]byte, error)
	templates  *template.Template
}

// NewServer constructs a Server with routes registered and ready to serve.
func NewServer(db *sql.DB, devicePath string, logger *slog.Logger) *Server {
	s := &Server{
		mux:        http.NewServeMux(),
		db:         db,
		devicePath: devicePath,
		logger:     logger,
		runAxicli:  runAxicliCmd,
		templates:  parseTemplates(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /sysinfo", s.handleSysinfo)
	s.mux.HandleFunc("POST /heartbeat", s.handleCreateHeartbeat)
	s.mux.HandleFunc("GET /heartbeat", s.handleListHeartbeats)

	s.mux.HandleFunc("GET /device-config", s.handleGetDeviceConfig)
	s.mux.HandleFunc("PUT /device-config", s.handleUpdateDeviceConfig)

	s.mux.HandleFunc("GET /presets", s.handleListPresets)
	s.mux.HandleFunc("POST /presets", s.handleCreatePreset)
	s.mux.HandleFunc("GET /presets/{id}", s.handleGetPreset)
	s.mux.HandleFunc("GET /presets/{id}/edit", s.handleEditPreset)
	s.mux.HandleFunc("PUT /presets/{id}", s.handleUpdatePreset)
	s.mux.HandleFunc("DELETE /presets/{id}", s.handleDeletePreset)

	s.mux.Handle("GET /static/", http.StripPrefix("/static/", staticHandler()))
	s.mux.HandleFunc("GET /{$}", s.handleIndex)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/device-config", http.StatusFound)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
