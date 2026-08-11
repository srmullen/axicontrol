package api

import (
	"net/http"
	"strconv"
)

// printPageView is the per-upload print page's data (ADR-0017): the Upload
// itself, the Presets available to submit a Job against it, whether its SVG
// has any discovered Layers (gating whether the mode selector appears at
// all), its currently in-progress Job if any, and an inline validation
// error from the last submission attempt.
type printPageView struct {
	Upload    uploadView
	Presets   []presetView
	HasLayers bool
	Job       *jobRowView
	FormError string
}

// handleUploadPrintPage renders the page scoped to a single Upload: its
// whole-document preview, its Job-submission form, and (if one is running)
// its in-progress Job's live status. This is the sole place a Job can be
// created (ADR-0017) — /jobs is pure cross-file history/monitoring.
func (s *Server) handleUploadPrintPage(w http.ResponseWriter, r *http.Request) {
	id, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	upload, ok := s.loadUploadOrNotFound(w, r, id)
	if !ok {
		return
	}

	view, err := s.loadPrintPageView(r, upload, "")
	if err != nil {
		s.logger.Error("load print page failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.renderPage(w, "print_content", view)
}

// loadPrintPageView assembles upload's print page view: its available
// Presets, whether its SVG has any discovered Layers (svg.DiscoverLayers),
// and its currently in-progress Job, if any (see loadInProgressJobForFile).
func (s *Server) loadPrintPageView(r *http.Request, upload uploadView, formErr string) (printPageView, error) {
	_, storageKey, err := s.loadFileRecord(r.Context(), upload.ID)
	if err != nil {
		return printPageView{}, err
	}

	layerNumbers, err := s.discoverLayersForFile(r.Context(), storageKey)
	if err != nil {
		return printPageView{}, err
	}

	presets, err := s.loadPresets(r)
	if err != nil {
		return printPageView{}, err
	}

	job, err := s.loadInProgressJobForFile(r.Context(), upload.ID)
	if err != nil {
		return printPageView{}, err
	}

	return printPageView{
		Upload:    upload,
		Presets:   presets,
		HasLayers: len(layerNumbers) > 0,
		Job:       job,
		FormError: formErr,
	}, nil
}

// rerenderPrintSection re-loads upload's print-page view and renders it as
// the #print-section fragment — the common tail for both a successful Job
// submission and a validation error, so the section always reflects
// current state, including any Job the submission just started.
func (s *Server) rerenderPrintSection(w http.ResponseWriter, r *http.Request, upload uploadView, formErr string) {
	view, err := s.loadPrintPageView(r, upload, formErr)
	if err != nil {
		s.logger.Error("load print page failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderFragment(w, http.StatusOK, "print_section", view)
}

// handleCreateJobForUpload creates a Job for the Upload named by the path
// (ADR-0017) — the print page's submission form, scoped to this Upload with
// no file picker. Preset and Plot Mode still come from the form; mode
// defaults to "whole" when absent (the mode selector itself is hidden by
// the template whenever the Upload has no discovered Layers).
func (s *Server) handleCreateJobForUpload(w http.ResponseWriter, r *http.Request) {
	fileID, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	upload, ok := s.loadUploadOrNotFound(w, r, fileID)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	presetID, err := strconv.ParseInt(r.FormValue("preset_id"), 10, 64)
	if err != nil {
		s.rerenderPrintSection(w, r, upload, "select a preset")
		return
	}

	ov, err := parseOverridesForm(r)
	if err != nil {
		s.rerenderPrintSection(w, r, upload, err.Error())
		return
	}

	_, userErr, err := s.createJobForFile(r.Context(), fileID, presetID, r.FormValue("mode"), ov)
	if err != nil {
		s.logger.Error("create job failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if userErr != "" {
		s.rerenderPrintSection(w, r, upload, userErr)
		return
	}

	s.rerenderPrintSection(w, r, upload, "")
}

// handleShowJobPrintStatus renders jobID's live-status fragment for the
// print page (see print_job_status), the poll target the fragment's own
// hx-trigger hits — the print page's counterpart to handleShowJobRow.
func (s *Server) handleShowJobPrintStatus(w http.ResponseWriter, r *http.Request) {
	id, err := jobIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid job id")
		return
	}

	v, ok := s.loadJobRowOrNotFound(w, r, id)
	if !ok {
		return
	}
	s.renderFragment(w, http.StatusOK, "print_job_status", v)
}
