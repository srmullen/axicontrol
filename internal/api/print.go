package api

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/srmullen/axicontrol/internal/svg"
)

// printPageView is the per-upload print page's data (ADR-0017): the Upload
// itself, its discovered Layers (read-only display, axicontrol-layer-labels
// — an empty slice also gates the mode selector's own visibility), the
// Presets available to submit a Job against it, its currently in-progress
// Job if any, an inline validation error from the last submission attempt,
// and the live-preview area's own selection state (Mode/LayerNumber — see
// print_preview) — zero-valued ("whole file", no layer chosen) on a fresh
// page load, populated from the request on the preview fragment's own
// reload (handleUploadPreview).
type printPageView struct {
	Upload      uploadView
	Layers      []layerView
	Presets     []presetView
	Job         *jobRowView
	FormError   string
	Mode        string
	LayerNumber *int
}

// PreviewSrc is the <img> source the live-preview area should show: the
// isolated-Layer endpoint once Single Layer mode has a chosen Layer (see
// svg.IsolateLayer, ADR-0010's <img>-only preview constraint), otherwise
// the whole-document endpoint — including Single Layer mode before a Layer
// has been picked, since there's nothing layer-specific to isolate yet.
func (v printPageView) PreviewSrc() string {
	if v.Mode == "single_layer" && v.LayerNumber != nil {
		return fmt.Sprintf("/uploads/%d/layers/%d/content", v.Upload.ID, *v.LayerNumber)
	}
	return fmt.Sprintf("/uploads/%d/content", v.Upload.ID)
}

// LayerSelected reports whether n is the live-preview area's currently
// chosen Layer, for the Layer <select>'s "selected" option (html/template
// calls a monadic method like a function from within a template action).
func (v printPageView) LayerSelected(n int) bool {
	return v.LayerNumber != nil && *v.LayerNumber == n
}

// layerView is one discovered Layer's read-only display row on the print
// page: its number, plus its raw label(s) (see svg.Layer) with the leading
// number stripped and comma-joined — e.g. "5-red"/"5-outlines" becomes
// "red, outlines" — since the number is already shown alongside it. Empty
// when none of the Layer's labels have any text beyond the bare number.
type layerView struct {
	Number int
	Labels string
}

// buildLayerViews converts DiscoverLayers's raw per-layer labels into their
// print-page display form (see layerView).
func buildLayerViews(layers []svg.Layer) []layerView {
	views := make([]layerView, len(layers))
	for i, l := range layers {
		stripped := make([]string, len(l.Labels))
		for j, label := range l.Labels {
			stripped[j] = svg.StripNumberPrefix(label)
		}
		views[i] = layerView{Number: l.Number, Labels: strings.Join(stripped, ", ")}
	}
	return views
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

	layers, err := s.discoverLayersForFile(r.Context(), storageKey)
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
		Layers:    buildLayerViews(layers),
		Presets:   presets,
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

	singleLayer, err := layerNumberFromForm(r)
	if err != nil {
		s.rerenderPrintSection(w, r, upload, err.Error())
		return
	}

	_, userErr, err := s.createJobForFile(r.Context(), fileID, presetID, r.FormValue("mode"), singleLayer, ov)
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

// layerNumberFromForm reads an already-parsed request form's optional
// "layer_number" field, shared by Single Layer mode's Job submission and
// its live-preview reload (handleUploadPreview). A blank field means "no
// Layer chosen"; a non-blank field that fails to parse returns an
// already-user-facing error message, matching parseOverridesForm's
// convention for this handler family.
func layerNumberFromForm(r *http.Request) (*int, error) {
	raw := r.FormValue("layer_number")
	if raw == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return nil, errors.New("layer number must be an integer")
	}
	return &n, nil
}

// handleUploadPreview re-renders the live-preview area (see print_preview)
// for mode/layer_number query params — the target of the Plot Mode and
// Layer <select>s' own hx-get, so choosing Single Layer or a Layer swaps
// the visible preview without submitting a Job (ADR-0017's Single Layer
// mode).
func (s *Server) handleUploadPreview(w http.ResponseWriter, r *http.Request) {
	id, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	upload, ok := s.loadUploadOrNotFound(w, r, id)
	if !ok {
		return
	}

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	layerNumber, err := layerNumberFromForm(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	view, err := s.loadPrintPageView(r, upload, "")
	if err != nil {
		s.logger.Error("load print page failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	view.Mode = r.FormValue("mode")
	// A layer_number that isn't actually one of this Upload's own
	// discovered Layers (a stale selection, or a hand-crafted request) is
	// treated the same as none chosen — falls back to the whole-file
	// image — rather than building an <img src> that 404s.
	if layerNumber != nil && slices.ContainsFunc(view.Layers, func(l layerView) bool { return l.Number == *layerNumber }) {
		view.LayerNumber = layerNumber
	}

	s.renderFragment(w, http.StatusOK, "print_preview", view)
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
