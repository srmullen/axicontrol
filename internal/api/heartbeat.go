package api

import (
	"net/http"
)

type heartbeat struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
}

// handleCreateHeartbeat writes a trivial row, proving the SQLite connection
// accepts writes and that the schema is in place.
func (s *Server) handleCreateHeartbeat(w http.ResponseWriter, r *http.Request) {
	res, err := s.db.ExecContext(r.Context(), "INSERT INTO heartbeats DEFAULT VALUES")
	if err != nil {
		s.logger.Error("insert heartbeat failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	id, err := res.LastInsertId()
	if err != nil {
		s.logger.Error("read heartbeat id failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var hb heartbeat
	row := s.db.QueryRowContext(r.Context(), "SELECT id, created_at FROM heartbeats WHERE id = ?", id)
	if err := row.Scan(&hb.ID, &hb.CreatedAt); err != nil {
		s.logger.Error("read heartbeat failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, hb)
}

// handleListHeartbeats reads back every row written so far — the read half of
// the write/read persistence check. Run before and after a process restart
// against the same db path, the same rows should come back both times.
func (s *Server) handleListHeartbeats(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT id, created_at FROM heartbeats ORDER BY id")
	if err != nil {
		s.logger.Error("list heartbeats failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer func() { _ = rows.Close() }()

	heartbeats := []heartbeat{}
	for rows.Next() {
		var hb heartbeat
		if err := rows.Scan(&hb.ID, &hb.CreatedAt); err != nil {
			s.logger.Error("scan heartbeat failed", "error", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		heartbeats = append(heartbeats, hb)
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("iterate heartbeats failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, heartbeats)
}
