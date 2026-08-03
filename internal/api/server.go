// Package api implements axicontrol's HTTP surface.
package api

import (
	"database/sql"
	"encoding/json"
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
}

// NewServer constructs a Server with routes registered and ready to serve.
func NewServer(db *sql.DB, devicePath string, logger *slog.Logger) *Server {
	s := &Server{
		mux:        http.NewServeMux(),
		db:         db,
		devicePath: devicePath,
		logger:     logger,
		runAxicli:  runAxicliCmd,
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /sysinfo", s.handleSysinfo)
	s.mux.HandleFunc("POST /heartbeat", s.handleCreateHeartbeat)
	s.mux.HandleFunc("GET /heartbeat", s.handleListHeartbeats)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
