package api

import (
	"net/http"
	"strconv"
)

// deviceConfigView holds the singleton Device Config's hardware-fixed values,
// plus per-request UI state (an error from the last save, or a success flag).
type deviceConfigView struct {
	Model   int
	Penlift int
	Error   string
	Saved   bool
}

func (s *Server) loadDeviceConfig(r *http.Request) (deviceConfigView, error) {
	var v deviceConfigView
	row := s.db.QueryRowContext(r.Context(), "SELECT model, penlift FROM device_config WHERE id = 1")
	err := row.Scan(&v.Model, &v.Penlift)
	return v, err
}

func (s *Server) handleGetDeviceConfig(w http.ResponseWriter, r *http.Request) {
	v, err := s.loadDeviceConfig(r)
	if err != nil {
		s.logger.Error("load device config failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.renderPage(w, "device_config_content", v)
}

func (s *Server) handleUpdateDeviceConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	model, modelErr := strconv.Atoi(r.FormValue("model"))
	penlift, penliftErr := strconv.Atoi(r.FormValue("penlift"))
	if modelErr != nil || penliftErr != nil {
		v := deviceConfigView{Error: "model and penlift must both be integers"}
		if modelErr == nil {
			v.Model = model
		}
		if penliftErr == nil {
			v.Penlift = penlift
		}
		// htmx only swaps a fragment into the DOM on a 2xx response by
		// default, so the inline error must ship as 200 to actually render.
		s.renderFragment(w, http.StatusOK, "device_config_form", v)
		return
	}

	_, err := s.db.ExecContext(r.Context(),
		"UPDATE device_config SET model = ?, penlift = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = 1",
		model, penlift,
	)
	if err != nil {
		s.logger.Error("update device config failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.renderFragment(w, http.StatusOK, "device_config_form", deviceConfigView{Model: model, Penlift: penlift, Saved: true})
}
