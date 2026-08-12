package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"

	"github.com/srmullen/axicontrol/internal/svg"
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

// jobRowView is a Job's list of Passes flattened into one row, reporting on
// whichever Pass is currently "active" (see activePass): Job status is
// derived from its Passes' statuses (ADR-0002), not tracked independently.
type jobRowView struct {
	ID         int64
	FileID     int64
	Filename   string
	PresetName string
	Status     string
	Output     string
	CreatedAt  string
	PassNumber int // 1-indexed position of the active Pass
	PassCount  int

	// OOB marks this row for an out-of-band swap (see writeJobUpdateEvent):
	// set only when rendering a row to push over SSE, never for a row
	// rendered as an HTTP response's own primary content.
	OOB bool
}

// Polling reports whether this row should keep htmx-polling itself for
// updates. Named for template use (html/template calls niladic methods).
func (v jobRowView) Polling() bool {
	return v.Status == "queued" || v.Status == "printing"
}

// InProgress reports whether this Job is still active — the set of
// statuses a per-upload print page shows inline (ADR-0017), as opposed to a
// terminal Job (complete/failed/cancelled) that's just history.
func (v jobRowView) InProgress() bool {
	switch v.Status {
	case "queued", "printing", "paused", "awaiting-next-pass":
		return true
	}
	return false
}

// passSummary is one Pass's state as needed to derive its Job's overall
// status and to report on whichever Pass is currently active.
type passSummary struct {
	ID            int64
	SequenceIndex int
	Status        string
	Output        string
	PresetName    string
}

const passSummaryQuery = `SELECT p.id, p.sequence_index, p.status, p.output, pr.name
	FROM passes p JOIN presets pr ON pr.id = p.preset_id
	WHERE p.job_id = ? ORDER BY p.sequence_index`

func (s *Server) loadPassSummariesForJob(ctx context.Context, jobID int64) ([]passSummary, error) {
	rows, err := s.db.QueryContext(ctx, passSummaryQuery, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var passes []passSummary
	for rows.Next() {
		var p passSummary
		if err := rows.Scan(&p.ID, &p.SequenceIndex, &p.Status, &p.Output, &p.PresetName); err != nil {
			return nil, err
		}
		passes = append(passes, p)
	}
	return passes, rows.Err()
}

// loadAllPassSummariesByJob loads every Pass across every Job in one query,
// grouped by job_id (each group ordered by sequence_index) — the bulk
// counterpart to loadPassSummariesForJob, used by loadJobs to avoid a
// per-Job round trip.
func (s *Server) loadAllPassSummariesByJob(ctx context.Context) (map[int64][]passSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.job_id, p.id, p.sequence_index, p.status, p.output, pr.name
		FROM passes p JOIN presets pr ON pr.id = p.preset_id
		ORDER BY p.job_id, p.sequence_index`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	byJob := map[int64][]passSummary{}
	for rows.Next() {
		var jobID int64
		var p passSummary
		if err := rows.Scan(&jobID, &p.ID, &p.SequenceIndex, &p.Status, &p.Output, &p.PresetName); err != nil {
			return nil, err
		}
		byJob[jobID] = append(byJob[jobID], p)
	}
	return byJob, rows.Err()
}

// deriveJobStatus derives a Job's overall status from its Passes (ordered by
// sequence_index), per ADR-0002: the first not-yet-complete Pass determines
// it — pending means "queued" if it's the very first Pass, or
// "awaiting-next-pass" if an earlier Pass already completed; running means
// "printing"; anything else (failed/cancelled/paused) surfaces as-is. All
// Passes complete means the Job is complete.
func deriveJobStatus(passes []passSummary) string {
	for i, p := range passes {
		switch p.Status {
		case "complete":
			continue
		case "pending":
			if i == 0 {
				return "queued"
			}
			return "awaiting-next-pass"
		case "running":
			return "printing"
		default:
			return p.Status
		}
	}
	return "complete"
}

// activePass returns the Pass a Job's status/output/retry/advance actions
// pertain to: the first not-yet-complete Pass, or the last Pass if every
// Pass is complete. Panics if passes is empty; callers only call this after
// confirming a Job has at least one Pass.
func activePass(passes []passSummary) passSummary {
	for _, p := range passes {
		if p.Status != "complete" {
			return p
		}
	}
	return passes[len(passes)-1]
}

type jobsSectionView struct {
	Jobs []jobRowView
}

// loadJobs loads every Job's row view in exactly two queries total (job
// cores, then all Passes grouped by job_id — see loadAllPassSummariesByJob),
// rather than one round trip per Job.
func (s *Server) loadJobs(ctx context.Context) ([]jobRowView, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT j.id, j.file_id, f.filename, j.created_at
		FROM jobs j JOIN files f ON f.id = j.file_id
		ORDER BY j.created_at DESC, j.id DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	type jobCore struct {
		id        int64
		fileID    int64
		filename  string
		createdAt string
	}
	var cores []jobCore
	for rows.Next() {
		var c jobCore
		if err := rows.Scan(&c.id, &c.fileID, &c.filename, &c.createdAt); err != nil {
			return nil, err
		}
		cores = append(cores, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	passesByJob, err := s.loadAllPassSummariesByJob(ctx)
	if err != nil {
		return nil, err
	}

	jobs := []jobRowView{}
	for _, c := range cores {
		passes := passesByJob[c.id]
		if len(passes) == 0 {
			continue // shouldn't happen: every Job has at least one Pass
		}
		jobs = append(jobs, buildJobRowView(c.id, c.fileID, c.filename, c.createdAt, passes))
	}
	return jobs, nil
}

// buildJobRowView assembles a Job's row view from its file info and its
// Passes' derived status, output, and progress (see deriveJobStatus,
// activePass). passes must be non-empty and ordered by sequence_index.
func buildJobRowView(id, fileID int64, filename, createdAt string, passes []passSummary) jobRowView {
	active := activePass(passes)
	return jobRowView{
		ID:         id,
		FileID:     fileID,
		Filename:   filename,
		CreatedAt:  createdAt,
		PresetName: active.PresetName,
		Status:     deriveJobStatus(passes),
		Output:     active.Output,
		PassNumber: active.SequenceIndex + 1,
		PassCount:  len(passes),
	}
}

// loadJobRow assembles a single Job's row view. Returns sql.ErrNoRows if id
// doesn't exist or (should never happen) has no Passes.
func (s *Server) loadJobRow(ctx context.Context, id int64) (jobRowView, error) {
	var fileID int64
	var filename, createdAt string
	row := s.db.QueryRowContext(ctx,
		"SELECT j.file_id, f.filename, j.created_at FROM jobs j JOIN files f ON f.id = j.file_id WHERE j.id = ?", id)
	if err := row.Scan(&fileID, &filename, &createdAt); err != nil {
		return jobRowView{}, err
	}

	passes, err := s.loadPassSummariesForJob(ctx, id)
	if err != nil {
		return jobRowView{}, err
	}
	if len(passes) == 0 {
		return jobRowView{}, sql.ErrNoRows
	}

	return buildJobRowView(id, fileID, filename, createdAt, passes), nil
}

// loadJobRowByQuery loads a single Job row view via query, which must select
// (job id, file_id, filename, created_at), in that order, and takes args
// positionally — the row-assembly shared by loadLatestJobForFile and
// loadLatestJob, which differ only in which Job that query picks out.
// Returns ok=false, no error, if query matches no row or that Job
// (shouldn't happen) has no Passes.
func (s *Server) loadJobRowByQuery(ctx context.Context, query string, args ...any) (jobRowView, bool, error) {
	var id, fileID int64
	var filename, createdAt string
	row := s.db.QueryRowContext(ctx, query, args...)
	if err := row.Scan(&id, &fileID, &filename, &createdAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return jobRowView{}, false, nil
		}
		return jobRowView{}, false, err
	}

	passes, err := s.loadPassSummariesForJob(ctx, id)
	if err != nil {
		return jobRowView{}, false, err
	}
	if len(passes) == 0 {
		return jobRowView{}, false, nil
	}

	return buildJobRowView(id, fileID, filename, createdAt, passes), true, nil
}

// loadLatestJobForFile loads fileID's most recently created Job, if any.
// It's the only Job that can possibly still be in progress: the
// device-busy guard (tryClaimDevice) never lets a second Job start against
// the same file while an earlier one is still active, so an older Job for
// the same file is always terminal by the time a newer one exists. Returns
// ok=false if fileID has no Jobs at all.
func (s *Server) loadLatestJobForFile(ctx context.Context, fileID int64) (jobRowView, bool, error) {
	return s.loadJobRowByQuery(ctx, `SELECT j.id, j.file_id, f.filename, j.created_at
		FROM jobs j JOIN files f ON f.id = j.file_id
		WHERE j.file_id = ? ORDER BY j.created_at DESC, j.id DESC LIMIT 1`, fileID)
}

// loadInProgressJobForFile loads fileID's currently in-progress Job (see
// jobRowView.InProgress), for a per-upload print page to show inline
// (ADR-0017). Returns a nil pointer, not an error, when fileID has no Job
// or its latest Job has already reached a terminal state.
func (s *Server) loadInProgressJobForFile(ctx context.Context, fileID int64) (*jobRowView, error) {
	row, ok, err := s.loadLatestJobForFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	if !ok || !row.InProgress() {
		return nil, nil
	}
	return &row, nil
}

// loadLatestJob loads the most recently created Job across every Upload, if
// any — loadLatestJobForFile without the file_id filter.
func (s *Server) loadLatestJob(ctx context.Context) (jobRowView, bool, error) {
	return s.loadJobRowByQuery(ctx, `SELECT j.id, j.file_id, f.filename, j.created_at
		FROM jobs j JOIN files f ON f.id = j.file_id
		ORDER BY j.created_at DESC, j.id DESC LIMIT 1`)
}

// loadBusyJobExcluding reports the Job currently claiming the device
// (deviceClaimed — see jobs_run.go), if any, unless it belongs to
// excludeFileID — a print page never needs to be told the device is busy
// with its own Upload's Job; that's already covered by its own in-progress
// Job section. Since tryClaimDevice never lets a second Job start while one
// is already in progress, at most one Job system-wide is ever in progress
// at a time, and it's always the most recently created one — so the busy
// Job, if any, is exactly loadLatestJob's result when it's still in
// progress.
func (s *Server) loadBusyJobExcluding(ctx context.Context, excludeFileID int64) (*jobRowView, error) {
	row, ok, err := s.loadLatestJob(ctx)
	if err != nil {
		return nil, err
	}
	if !ok || !row.InProgress() || row.FileID == excludeFileID {
		return nil, nil
	}
	return &row, nil
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

// loadActivePassForJobOrNotFound loads a job's active Pass (see activePass)
// along with the job's own derived status, writing the appropriate error
// response itself (404 or 500) and returning ok=false when there's nothing
// for the caller to act on.
func (s *Server) loadActivePassForJobOrNotFound(w http.ResponseWriter, r *http.Request, jobID int64) (pass passSummary, jobStatus string, ok bool) {
	passes, err := s.loadPassSummariesForJob(r.Context(), jobID)
	if err != nil {
		s.logger.Error("load passes for job failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return passSummary{}, "", false
	}
	if len(passes) == 0 {
		writeError(w, http.StatusNotFound, "job not found")
		return passSummary{}, "", false
	}
	return activePass(passes), deriveJobStatus(passes), true
}

func (s *Server) loadJobsSectionView(r *http.Request) (jobsSectionView, error) {
	jobs, err := s.loadJobs(r.Context())
	if err != nil {
		return jobsSectionView{}, err
	}
	return jobsSectionView{Jobs: jobs}, nil
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	view, err := s.loadJobsSectionView(r)
	if err != nil {
		s.logger.Error("list jobs failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderPage(w, "jobs_content", view)
}

// createJobForFile validates and creates a Job for fileID (an Upload's
// file id) and Presets it against presetID, starting its first Pass in the
// background on success. Shared by the print page's job-creation endpoint
// (ADR-0017); exactly one of its three return values is meaningful: a
// successfully started firstPassID, a user-facing validation message (e.g.
// "no numbered layers found") the caller should show inline instead of
// creating anything, or a genuine error the caller should treat as a 500.
// singleLayer is only consulted when mode is "single_layer" — the one Layer
// number the resulting Job's lone Pass plots, distinct from "layers" mode's
// one-Pass-per-discovered-Layer sequencing (CONTEXT.md's Plot Mode entry).
func (s *Server) createJobForFile(ctx context.Context, fileID, presetID int64, mode string, singleLayer *int, ov overrides) (firstPassID int64, userErr string, err error) {
	if mode == "" {
		mode = "whole"
	}
	if mode != "whole" && mode != "layers" && mode != "single_layer" {
		return 0, "invalid mode", nil
	}

	_, storageKey, err := s.loadFileRecord(ctx, fileID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "selected file not found", nil
	} else if err != nil {
		return 0, "", err
	}

	if _, err := s.loadPreset(ctx, presetID); errors.Is(err, sql.ErrNoRows) {
		return 0, "selected preset not found", nil
	} else if err != nil {
		return 0, "", err
	}

	var layerNumbers []int
	switch mode {
	case "layers":
		layers, err := s.discoverLayersForFile(ctx, storageKey)
		if err != nil {
			return 0, "", err
		}
		if len(layers) == 0 {
			return 0, "no numbered layers found in this SVG", nil
		}
		layerNumbers = svg.Numbers(layers)
	case "single_layer":
		if singleLayer == nil {
			return 0, "select a layer", nil
		}
		layers, err := s.discoverLayersForFile(ctx, storageKey)
		if err != nil {
			return 0, "", err
		}
		if !slices.Contains(svg.Numbers(layers), *singleLayer) {
			return 0, "layer not found", nil
		}
		layerNumbers = []int{*singleLayer}
	}

	if !s.tryClaimDevice() {
		return 0, deviceBusyMessage, nil
	}

	firstPassID, err = s.insertJobAndPasses(ctx, fileID, presetID, mode, layerNumbers, ov)
	if err != nil {
		s.releaseDevice(true)
		return 0, "", err
	}

	go s.executePass(firstPassID)
	return firstPassID, "", nil
}

// discoverLayersForFile reads storageKey's sanitized SVG content and returns
// its auto-discovered Layers (see svg.DiscoverLayers) — each layer number
// together with its raw label text(s) — for a layers-mode Job submission
// and for the print page's read-only layer list (axicontrol-layer-labels).
func (s *Server) discoverLayersForFile(ctx context.Context, storageKey string) ([]svg.Layer, error) {
	rc, err := s.files.Get(storageKey)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	return svg.DiscoverLayers(data)
}

// passLayerNumbersFor returns one *int per Pass to insert, in order: nil for
// whole mode's single unnumbered Pass (layerNumbers empty), or one pointer
// per entry in layerNumbers otherwise — one per discovered layer for layers
// mode, or the single chosen layer for single_layer mode.
func passLayerNumbersFor(layerNumbers []int) []*int {
	if len(layerNumbers) == 0 {
		return []*int{nil}
	}
	numbers := make([]*int, len(layerNumbers))
	for i := range layerNumbers {
		numbers[i] = &layerNumbers[i]
	}
	return numbers
}

// insertJobAndPasses creates a Job of the given mode and one Pass per entry
// in layerNumbers, in order (layerNumbers is ignored for whole mode, which
// always gets exactly one Pass with no layer number). Every Pass shares the
// same Preset + overrides — layers mode reuses whole-mode's config
// resolution unchanged per Pass rather than accepting a distinct config per
// layer. Returns the id of the first Pass, the one that starts immediately
// on submission.
func (s *Server) insertJobAndPasses(ctx context.Context, fileID, presetID int64, mode string, layerNumbers []int, ov overrides) (int64, error) {
	overridesJSON, err := json.Marshal(ov)
	if err != nil {
		return 0, fmt.Errorf("marshal overrides: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, "INSERT INTO jobs (file_id, mode) VALUES (?, ?)", fileID, mode)
	if err != nil {
		return 0, err
	}
	jobID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	var firstPassID int64
	for i, layerNumber := range passLayerNumbersFor(layerNumbers) {
		res, err = tx.ExecContext(ctx, `INSERT INTO passes (job_id, sequence_index, preset_id, overrides, status, layer_number)
			VALUES (?, ?, ?, ?, 'pending', ?)`, jobID, i, presetID, string(overridesJSON), layerNumber)
		if err != nil {
			return 0, err
		}
		passID, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		if i == 0 {
			firstPassID = passID
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return firstPassID, nil
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

	s.renderCurrentJobRow(w, r, id)
}

// loadJobActionTarget is the shared preamble for every Job action handler
// (retry/advance/pause/resume/cancel): parse the path's job id, then load
// its active Pass and derived Job status. Writes the appropriate error
// response itself and returns ok=false when there's nothing for the caller
// to act on.
func (s *Server) loadJobActionTarget(w http.ResponseWriter, r *http.Request) (jobID int64, pass passSummary, jobStatus string, ok bool) {
	jobID, err := jobIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return 0, passSummary{}, "", false
	}

	pass, jobStatus, ok = s.loadActivePassForJobOrNotFound(w, r, jobID)
	if !ok {
		return 0, passSummary{}, "", false
	}
	return jobID, pass, jobStatus, true
}

func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	id, pass, jobStatus, ok := s.loadJobActionTarget(w, r)
	if !ok {
		return
	}

	if jobStatus != "failed" {
		writeError(w, http.StatusConflict, "only a failed job can be retried")
		return
	}

	if !s.tryClaimDevice() {
		writeError(w, http.StatusConflict, deviceBusyMessage)
		return
	}

	if err := s.setPassStatus(r.Context(), pass.ID, "pending", ""); err != nil {
		s.releaseDevice(true)
		s.logger.Error("reset pass for retry failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.startPassAndRenderRow(w, r, id, pass.ID)
}

// handleAdvanceJob starts a layers-mode Job's next Pass. Valid only from
// awaiting-next-pass — a Pass completing never auto-starts the next one
// (ADR-0002): this is the explicit user trigger that does. Unlike Job
// creation/handleRetryJob, this doesn't reclaim the device: it stays
// claimed by this Job across the awaiting-next-pass gap (see
// Server.releaseDevice) precisely so an unrelated Job can't interleave a
// Pass on the same still-mounted artwork between layers.
func (s *Server) handleAdvanceJob(w http.ResponseWriter, r *http.Request) {
	id, pass, jobStatus, ok := s.loadJobActionTarget(w, r)
	if !ok {
		return
	}

	if jobStatus != "awaiting-next-pass" {
		writeError(w, http.StatusConflict, "job is not awaiting its next pass")
		return
	}

	if !s.tryStartNextPass() {
		writeError(w, http.StatusConflict, "this job's next pass is already starting")
		return
	}

	s.startPassAndRenderRow(w, r, id, pass.ID)
}

// startPassAndRenderRow launches passID's execution in the background and
// renders jobID's row fragment for the htmx response that triggered it —
// the common tail shared by retry, advance, and resume.
func (s *Server) startPassAndRenderRow(w http.ResponseWriter, r *http.Request, jobID, passID int64) {
	go s.executePass(passID)

	v, ok := s.loadJobRowOrNotFound(w, r, jobID)
	if !ok {
		return
	}
	s.renderFragment(w, http.StatusOK, "job_row", v)
}

// renderCurrentJobRow re-loads and renders jobID's row fragment as-is,
// without starting anything — the common tail for pause and the
// synchronous branch of cancel, which only change state a running Pass's
// own goroutine (or, for cancel, this handler itself) already wrote.
func (s *Server) renderCurrentJobRow(w http.ResponseWriter, r *http.Request, jobID int64) {
	v, ok := s.loadJobRowOrNotFound(w, r, jobID)
	if !ok {
		return
	}
	s.renderFragment(w, http.StatusOK, "job_row", v)
}

// handlePauseJob asks the active Pass's running axicli subprocess to stop
// (SIGINT via requestInterrupt): it finishes its current line segment,
// writes its checkpoint, and exits (ADR-0002). That happens on the Pass's
// own goroutine, not synchronously here, so the row rendered back still
// reads "printing" until a subsequent poll picks up "paused".
func (s *Server) handlePauseJob(w http.ResponseWriter, r *http.Request) {
	id, pass, jobStatus, ok := s.loadJobActionTarget(w, r)
	if !ok {
		return
	}

	if jobStatus != "printing" {
		writeError(w, http.StatusConflict, "job is not currently printing")
		return
	}

	if !s.requestInterrupt(pass.ID, "paused") {
		writeError(w, http.StatusConflict, "pass is no longer running")
		return
	}

	s.renderCurrentJobRow(w, r, id)
}

// handleResumeJob resumes a paused Pass from its checkpoint (res_plot,
// per ADR-0002). The device is still claimed by this Job from when it
// paused (see Server.releaseDevice), so this only needs tryStartNextPass's
// finer-grained passRunning guard — the same one advance uses — to prevent
// double-starting it.
func (s *Server) handleResumeJob(w http.ResponseWriter, r *http.Request) {
	id, pass, jobStatus, ok := s.loadJobActionTarget(w, r)
	if !ok {
		return
	}

	if jobStatus != "paused" {
		writeError(w, http.StatusConflict, "job is not paused")
		return
	}

	if !s.tryStartNextPass() {
		writeError(w, http.StatusConflict, "this job's pass is already starting")
		return
	}

	s.startPassAndRenderRow(w, r, id, pass.ID)
}

// handleCancelJob cancels a Job from pending, paused, or running (ADR-0002)
// — cancelled is terminal either way. A running Pass has no synchronous
// state to change here: it's interrupted the same way pause is, landing on
// "cancelled" instead of "paused" once axicli actually exits (see
// executePass). A pending or paused Pass has no subprocess to stop, so it's
// cancelled directly, including its checkpoint's deletion (a paused Pass
// has one; a pending one may not, but Delete is delete-if-exists).
func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id, pass, jobStatus, ok := s.loadJobActionTarget(w, r)
	if !ok {
		return
	}

	switch jobStatus {
	case "printing":
		if !s.requestInterrupt(pass.ID, "cancelled") {
			writeError(w, http.StatusConflict, "pass is no longer running")
			return
		}
	case "queued", "awaiting-next-pass", "paused":
		cancelled, err := s.tryCancelIfNotRunning(r.Context(), pass.ID)
		if err != nil {
			s.logger.Error("cancel pass failed", "error", err)
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !cancelled {
			// Lost a race: the Pass started running (e.g. via resume)
			// between loadActivePassForJobOrNotFound above and this
			// attempt — interrupt it instead, same as the "printing" case.
			if !s.requestInterrupt(pass.ID, "cancelled") {
				writeError(w, http.StatusConflict, "pass is no longer running")
				return
			}
			break
		}
		s.deleteCheckpoint(pass.ID)
		s.releaseDevice(true)
	default:
		writeError(w, http.StatusConflict, "job cannot be cancelled from its current state")
		return
	}

	s.renderCurrentJobRow(w, r, id)
}
