package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// overrides holds an optional per-Pass key/value override layered on top of
// a Preset, mirroring axicli's own args-override-config-file precedence
// (ADR-0003) without forcing a new named Preset for a one-off tweak. A nil
// field means "use the Preset's own value."
type overrides struct {
	SpeedPendown *int  `json:"speed_pendown,omitempty"`
	SpeedPenup   *int  `json:"speed_penup,omitempty"`
	Accel        *int  `json:"accel,omitempty"`
	PenPosDown   *int  `json:"pen_pos_down,omitempty"`
	PenPosUp     *int  `json:"pen_pos_up,omitempty"`
	PenRateLower *int  `json:"pen_rate_lower,omitempty"`
	PenRateRaise *int  `json:"pen_rate_raise,omitempty"`
	PenDelayDown *int  `json:"pen_delay_down,omitempty"`
	PenDelayUp   *int  `json:"pen_delay_up,omitempty"`
	ConstSpeed   *bool `json:"const_speed,omitempty"`
}

// apply resolves base (a Preset's plot-affecting values) with o layered on
// top, returning the single config that gets passed to axicli.
func (o overrides) apply(base presetView) presetView {
	resolved := base
	if o.SpeedPendown != nil {
		resolved.SpeedPendown = *o.SpeedPendown
	}
	if o.SpeedPenup != nil {
		resolved.SpeedPenup = *o.SpeedPenup
	}
	if o.Accel != nil {
		resolved.Accel = *o.Accel
	}
	if o.PenPosDown != nil {
		resolved.PenPosDown = *o.PenPosDown
	}
	if o.PenPosUp != nil {
		resolved.PenPosUp = *o.PenPosUp
	}
	if o.PenRateLower != nil {
		resolved.PenRateLower = *o.PenRateLower
	}
	if o.PenRateRaise != nil {
		resolved.PenRateRaise = *o.PenRateRaise
	}
	if o.PenDelayDown != nil {
		resolved.PenDelayDown = *o.PenDelayDown
	}
	if o.PenDelayUp != nil {
		resolved.PenDelayUp = *o.PenDelayUp
	}
	if o.ConstSpeed != nil {
		resolved.ConstSpeed = *o.ConstSpeed
	}
	return resolved
}

// parseOverridesForm reads optional per-field overrides out of an
// already-parsed request form. A blank field means "no override"; a
// non-blank field that fails to parse is a validation error.
func parseOverridesForm(r *http.Request) (overrides, error) {
	var ov overrides

	intFields := []struct {
		name string
		dst  **int
	}{
		{"speed_pendown", &ov.SpeedPendown},
		{"speed_penup", &ov.SpeedPenup},
		{"accel", &ov.Accel},
		{"pen_pos_down", &ov.PenPosDown},
		{"pen_pos_up", &ov.PenPosUp},
		{"pen_rate_lower", &ov.PenRateLower},
		{"pen_rate_raise", &ov.PenRateRaise},
		{"pen_delay_down", &ov.PenDelayDown},
		{"pen_delay_up", &ov.PenDelayUp},
	}

	for _, f := range intFields {
		raw := r.FormValue(f.name)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return overrides{}, fmt.Errorf("%s override must be an integer", f.name)
		}
		*f.dst = &n
	}

	switch raw := r.FormValue("const_speed"); raw {
	case "":
	case "true":
		v := true
		ov.ConstSpeed = &v
	case "false":
		v := false
		ov.ConstSpeed = &v
	default:
		return overrides{}, fmt.Errorf("const_speed override must be true, false, or blank")
	}

	return ov, nil
}

// jobRowView is a whole-mode Job's single Pass flattened into one row: Job
// status is derived from Pass status (ADR-0002), not tracked independently.
type jobRowView struct {
	ID         int64
	Filename   string
	PresetName string
	Status     string
	Output     string
	CreatedAt  string
}

// Polling reports whether this row should keep htmx-polling itself for
// updates. Named for template use (html/template calls niladic methods).
func (v jobRowView) Polling() bool {
	return v.Status == "queued" || v.Status == "printing"
}

func passStatusToJobStatus(passStatus string) string {
	switch passStatus {
	case "pending":
		return "queued"
	case "running":
		return "printing"
	default:
		return passStatus // complete, failed
	}
}

type newJobFormView struct {
	Files   []uploadView
	Presets []presetView
	Error   string
}

type jobsSectionView struct {
	Jobs []jobRowView
	Form newJobFormView
}

const jobRowQuery = `SELECT j.id, f.filename, pr.name, p.status, p.output, j.created_at
	FROM jobs j
	JOIN files f ON f.id = j.file_id
	JOIN passes p ON p.job_id = j.id AND p.sequence_index = 0
	JOIN presets pr ON pr.id = p.preset_id`

func scanJobRow(row interface{ Scan(...any) error }) (jobRowView, error) {
	var v jobRowView
	var passStatus string
	err := row.Scan(&v.ID, &v.Filename, &v.PresetName, &passStatus, &v.Output, &v.CreatedAt)
	v.Status = passStatusToJobStatus(passStatus)
	return v, err
}

func (s *Server) loadJobs(ctx context.Context) ([]jobRowView, error) {
	rows, err := s.db.QueryContext(ctx, jobRowQuery+" ORDER BY j.created_at DESC, j.id DESC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	jobs := []jobRowView{}
	for rows.Next() {
		v, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, v)
	}
	return jobs, rows.Err()
}

func (s *Server) loadJobRow(ctx context.Context, id int64) (jobRowView, error) {
	row := s.db.QueryRowContext(ctx, jobRowQuery+" WHERE j.id = ?", id)
	return scanJobRow(row)
}

// loadJobRowOrNotFound loads a job row by id, writing the appropriate error
// response itself (404 or 500) and returning ok=false when there's nothing
// for the caller to render.
func (s *Server) loadJobRowOrNotFound(w http.ResponseWriter, r *http.Request, id int64) (jobRowView, bool) {
	v, err := s.loadJobRow(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "job not found")
		return jobRowView{}, false
	}
	if err != nil {
		s.logger.Error("load job failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return jobRowView{}, false
	}
	return v, true
}

func (s *Server) loadFileRecord(ctx context.Context, id int64) (filename, storageKey string, err error) {
	row := s.db.QueryRowContext(ctx, "SELECT filename, storage_key FROM files WHERE id = ?", id)
	err = row.Scan(&filename, &storageKey)
	return filename, storageKey, err
}

func (s *Server) setPassStatus(ctx context.Context, id int64, status, output string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE passes SET status = ?, output = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		status, output, id)
	return err
}

func (s *Server) loadPassIDAndStatusForJob(ctx context.Context, jobID int64) (id int64, status string, err error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, status FROM passes WHERE job_id = ? AND sequence_index = 0", jobID)
	err = row.Scan(&id, &status)
	return id, status, err
}

// loadPassForJobOrNotFound loads a job's single Pass id and status, writing
// the appropriate error response itself (404 or 500) and returning ok=false
// when there's nothing for the caller to act on.
func (s *Server) loadPassForJobOrNotFound(w http.ResponseWriter, r *http.Request, jobID int64) (passID int64, status string, ok bool) {
	passID, status, err := s.loadPassIDAndStatusForJob(r.Context(), jobID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "job not found")
		return 0, "", false
	}
	if err != nil {
		s.logger.Error("load job for retry failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return 0, "", false
	}
	return passID, status, true
}

func (s *Server) loadJobsSectionView(r *http.Request, formErr string) (jobsSectionView, error) {
	jobs, err := s.loadJobs(r.Context())
	if err != nil {
		return jobsSectionView{}, err
	}
	files, err := s.loadUploads(r)
	if err != nil {
		return jobsSectionView{}, err
	}
	presets, err := s.loadPresets(r)
	if err != nil {
		return jobsSectionView{}, err
	}
	return jobsSectionView{Jobs: jobs, Form: newJobFormView{Files: files, Presets: presets, Error: formErr}}, nil
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	view, err := s.loadJobsSectionView(r, "")
	if err != nil {
		s.logger.Error("list jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderPage(w, "jobs_content", view)
}

// rerenderJobsSection re-loads the job list and renders it plus the new-job
// form (with formErr, if any) as the #jobs-section fragment.
func (s *Server) rerenderJobsSection(w http.ResponseWriter, r *http.Request, status int, formErr string) {
	view, err := s.loadJobsSectionView(r, formErr)
	if err != nil {
		s.logger.Error("list jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderFragment(w, status, "jobs_section", view)
}

func (s *Server) handleCreateJob(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fileID, err := strconv.ParseInt(r.FormValue("file_id"), 10, 64)
	if err != nil {
		s.rerenderJobsSection(w, r, http.StatusOK, "select a file")
		return
	}
	presetID, err := strconv.ParseInt(r.FormValue("preset_id"), 10, 64)
	if err != nil {
		s.rerenderJobsSection(w, r, http.StatusOK, "select a preset")
		return
	}

	ov, err := parseOverridesForm(r)
	if err != nil {
		s.rerenderJobsSection(w, r, http.StatusOK, err.Error())
		return
	}

	if _, _, err := s.loadFileRecord(r.Context(), fileID); errors.Is(err, sql.ErrNoRows) {
		s.rerenderJobsSection(w, r, http.StatusOK, "selected file not found")
		return
	} else if err != nil {
		s.logger.Error("load file for job failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if _, err := s.loadPreset(r.Context(), presetID); errors.Is(err, sql.ErrNoRows) {
		s.rerenderJobsSection(w, r, http.StatusOK, "selected preset not found")
		return
	} else if err != nil {
		s.logger.Error("load preset for job failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !s.tryStartPrinting() {
		s.rerenderJobsSection(w, r, http.StatusOK, "a job is already printing; wait for it to finish")
		return
	}

	passID, err := s.insertJobAndPass(r.Context(), fileID, presetID, ov)
	if err != nil {
		s.finishPrinting()
		s.logger.Error("create job failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go s.executePass(passID)

	s.rerenderJobsSection(w, r, http.StatusOK, "")
}

func (s *Server) insertJobAndPass(ctx context.Context, fileID, presetID int64, ov overrides) (int64, error) {
	overridesJSON, err := json.Marshal(ov)
	if err != nil {
		return 0, fmt.Errorf("marshal overrides: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, "INSERT INTO jobs (file_id, mode) VALUES (?, 'whole')", fileID)
	if err != nil {
		return 0, err
	}
	jobID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	res, err = tx.ExecContext(ctx, `INSERT INTO passes (job_id, sequence_index, preset_id, overrides, status)
		VALUES (?, 0, ?, ?, 'pending')`, jobID, presetID, string(overridesJSON))
	if err != nil {
		return 0, err
	}
	passID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return passID, nil
}

func jobIDFromPath(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (s *Server) handleShowJobRow(w http.ResponseWriter, r *http.Request) {
	id, err := jobIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	v, ok := s.loadJobRowOrNotFound(w, r, id)
	if !ok {
		return
	}

	s.renderFragment(w, http.StatusOK, "job_row", v)
}

func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	id, err := jobIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	passID, status, ok := s.loadPassForJobOrNotFound(w, r, id)
	if !ok {
		return
	}

	if status != "failed" {
		writeError(w, http.StatusConflict, "only a failed job can be retried")
		return
	}

	if !s.tryStartPrinting() {
		writeError(w, http.StatusConflict, "a job is already printing; wait for it to finish")
		return
	}

	if err := s.setPassStatus(r.Context(), passID, "pending", ""); err != nil {
		s.finishPrinting()
		s.logger.Error("reset pass for retry failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	go s.executePass(passID)

	v, ok := s.loadJobRowOrNotFound(w, r, id)
	if !ok {
		return
	}
	s.renderFragment(w, http.StatusOK, "job_row", v)
}
