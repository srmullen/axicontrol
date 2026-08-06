package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// presetView is a Preset's plot-affecting values, plus per-request UI state
// (an inline error from the last create/update attempt). It doubles as the
// shape used to render both a read-only table row and an editable form,
// since the two differ only in which template consumes it.
type presetView struct {
	ID           int64
	Name         string
	Description  string
	SpeedPendown int
	SpeedPenup   int
	Accel        int
	PenPosDown   int
	PenPosUp     int
	PenRateLower int
	PenRateRaise int
	PenDelayDown int
	PenDelayUp   int
	ConstSpeed   bool
	Error        string
}

type presetsSectionView struct {
	Presets []presetView
	NewForm presetView
}

func newPresetFormDefaults() presetView {
	return presetView{
		SpeedPendown: 25,
		SpeedPenup:   75,
		Accel:        75,
		PenPosDown:   40,
		PenPosUp:     60,
		PenRateLower: 50,
		PenRateRaise: 50,
	}
}

const presetColumns = `id, name, description, speed_pendown, speed_penup, accel,
	pen_pos_down, pen_pos_up, pen_rate_lower, pen_rate_raise, pen_delay_down, pen_delay_up, const_speed`

func scanPreset(row interface{ Scan(...any) error }) (presetView, error) {
	var v presetView
	var constSpeed int
	err := row.Scan(&v.ID, &v.Name, &v.Description, &v.SpeedPendown, &v.SpeedPenup, &v.Accel,
		&v.PenPosDown, &v.PenPosUp, &v.PenRateLower, &v.PenRateRaise, &v.PenDelayDown, &v.PenDelayUp, &constSpeed)
	v.ConstSpeed = constSpeed != 0
	return v, err
}

func (s *Server) loadPresets(r *http.Request) ([]presetView, error) {
	rows, err := s.db.QueryContext(r.Context(), "SELECT "+presetColumns+" FROM presets ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	presets := []presetView{}
	for rows.Next() {
		v, err := scanPreset(rows)
		if err != nil {
			return nil, err
		}
		presets = append(presets, v)
	}
	return presets, rows.Err()
}

func (s *Server) loadPreset(ctx context.Context, id int64) (presetView, error) {
	row := s.db.QueryRowContext(ctx, "SELECT "+presetColumns+" FROM presets WHERE id = ?", id)
	return scanPreset(row)
}

// parsePresetForm reads the plot-affecting fields out of an already-parsed
// request form. On a bad integer field, it still returns as much of the form
// as parsed successfully so the caller can re-render with the user's input
// intact alongside the error.
func parsePresetForm(r *http.Request) (presetView, error) {
	v := presetView{
		Name:        r.FormValue("name"),
		Description: r.FormValue("description"),
		ConstSpeed:  r.FormValue("const_speed") != "",
	}

	fields := []struct {
		name string
		dst  *int
	}{
		{"speed_pendown", &v.SpeedPendown},
		{"speed_penup", &v.SpeedPenup},
		{"accel", &v.Accel},
		{"pen_pos_down", &v.PenPosDown},
		{"pen_pos_up", &v.PenPosUp},
		{"pen_rate_lower", &v.PenRateLower},
		{"pen_rate_raise", &v.PenRateRaise},
		{"pen_delay_down", &v.PenDelayDown},
		{"pen_delay_up", &v.PenDelayUp},
	}

	for _, f := range fields {
		n, err := strconv.Atoi(r.FormValue(f.name))
		if err != nil {
			return v, errors.New(f.name + " must be an integer")
		}
		*f.dst = n
	}

	if v.Name == "" {
		return v, errors.New("name is required")
	}

	return v, nil
}

func isUniqueConstraintErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Server) handleListPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := s.loadPresets(r)
	if err != nil {
		s.logger.Error("list presets failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.renderPage(w, "presets_content", presetsSectionView{Presets: presets, NewForm: newPresetFormDefaults()})
}

func (s *Server) handleCreatePreset(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	form, err := parsePresetForm(r)
	if err != nil {
		form.Error = err.Error()
		// htmx only swaps a fragment into the DOM on a 2xx response by
		// default, so the inline error must ship as 200 to actually render.
		s.rerenderPresetsSection(w, r, http.StatusOK, form)
		return
	}

	_, err = s.db.ExecContext(r.Context(), `INSERT INTO presets
		(name, description, speed_pendown, speed_penup, accel, pen_pos_down, pen_pos_up,
		 pen_rate_lower, pen_rate_raise, pen_delay_down, pen_delay_up, const_speed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		form.Name, form.Description, form.SpeedPendown, form.SpeedPenup, form.Accel,
		form.PenPosDown, form.PenPosUp, form.PenRateLower, form.PenRateRaise,
		form.PenDelayDown, form.PenDelayUp, boolToInt(form.ConstSpeed),
	)
	if isUniqueConstraintErr(err) {
		form.Error = "a preset named \"" + form.Name + "\" already exists"
		s.rerenderPresetsSection(w, r, http.StatusOK, form)
		return
	}
	if err != nil {
		s.logger.Error("create preset failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.rerenderPresetsSection(w, r, http.StatusOK, newPresetFormDefaults())
}

// rerenderPresetsSection re-loads the full preset list and renders it plus
// the new-preset form (populated with newForm, e.g. to show a validation
// error) as the #presets-section fragment.
func (s *Server) rerenderPresetsSection(w http.ResponseWriter, r *http.Request, status int, newForm presetView) {
	presets, err := s.loadPresets(r)
	if err != nil {
		s.logger.Error("list presets failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderFragment(w, status, "presets_section", presetsSectionView{Presets: presets, NewForm: newForm})
}

func presetIDFromPath(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// loadPresetOrNotFound loads a preset by id, writing the appropriate error
// response itself (404 or 500) and returning ok=false when there's nothing
// for the caller to render.
func (s *Server) loadPresetOrNotFound(w http.ResponseWriter, r *http.Request, id int64) (presetView, bool) {
	v, err := s.loadPreset(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "preset not found")
		return presetView{}, false
	}
	if err != nil {
		s.logger.Error("load preset failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return presetView{}, false
	}
	return v, true
}

func (s *Server) handleGetPreset(w http.ResponseWriter, r *http.Request) {
	id, err := presetIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid preset id")
		return
	}

	v, ok := s.loadPresetOrNotFound(w, r, id)
	if !ok {
		return
	}

	s.renderFragment(w, http.StatusOK, "preset_row", v)
}

func (s *Server) handleEditPreset(w http.ResponseWriter, r *http.Request) {
	id, err := presetIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid preset id")
		return
	}

	v, ok := s.loadPresetOrNotFound(w, r, id)
	if !ok {
		return
	}

	s.renderFragment(w, http.StatusOK, "preset_edit_row", v)
}

func (s *Server) handleUpdatePreset(w http.ResponseWriter, r *http.Request) {
	id, err := presetIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid preset id")
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	form, err := parsePresetForm(r)
	form.ID = id
	if err != nil {
		form.Error = err.Error()
		// htmx only swaps a fragment into the DOM on a 2xx response by
		// default, so the inline error must ship as 200 to actually render.
		s.renderFragment(w, http.StatusOK, "preset_edit_row", form)
		return
	}

	res, err := s.db.ExecContext(r.Context(), `UPDATE presets SET
		name = ?, description = ?, speed_pendown = ?, speed_penup = ?, accel = ?,
		pen_pos_down = ?, pen_pos_up = ?, pen_rate_lower = ?, pen_rate_raise = ?,
		pen_delay_down = ?, pen_delay_up = ?, const_speed = ?,
		updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		form.Name, form.Description, form.SpeedPendown, form.SpeedPenup, form.Accel,
		form.PenPosDown, form.PenPosUp, form.PenRateLower, form.PenRateRaise,
		form.PenDelayDown, form.PenDelayUp, boolToInt(form.ConstSpeed), id,
	)
	if isUniqueConstraintErr(err) {
		form.Error = "a preset named \"" + form.Name + "\" already exists"
		s.renderFragment(w, http.StatusOK, "preset_edit_row", form)
		return
	}
	if err != nil {
		s.logger.Error("update preset failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "preset not found")
		return
	}

	s.renderFragment(w, http.StatusOK, "preset_row", form)
}

func (s *Server) handleDeletePreset(w http.ResponseWriter, r *http.Request) {
	id, err := presetIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid preset id")
		return
	}

	res, err := s.db.ExecContext(r.Context(), "DELETE FROM presets WHERE id = ?", id)
	if err != nil {
		s.logger.Error("delete preset failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "preset not found")
		return
	}

	w.WriteHeader(http.StatusOK)
}
