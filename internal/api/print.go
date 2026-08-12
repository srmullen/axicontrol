package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/srmullen/axicontrol/internal/svg"
)

// printPageView is the per-upload print page's data (ADR-0017): the Upload
// itself, its discovered Layers (svg.DiscoverLayers — gates the mode
// selector's own visibility, and supplies the Layer <select>'s options and
// the layers-pause status card's labels; not rendered as a list of its own
// on the page), the Presets available to submit a Job against it, its
// currently in-progress Job if any, the AxiDraw's device-busy Job if it's a
// *different* Upload's (axicontrol-device-busy-visibility — nil both when
// the device is free and when the busy Job is this page's own, already
// covered by Job above), an inline validation error from the last
// submission attempt, and the live-preview area's own selection state
// (Mode/LayerNumber — see print_preview) — zero-valued ("whole file", no
// layer chosen) on a fresh page load, populated from the request on the
// preview fragment's own reload (handleUploadPreview).
type printPageView struct {
	Upload      uploadView
	Layers      []layerView
	Presets     []presetView
	Job         *printJobStatusView
	BusyJob     *jobRowView
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

// printJobStatusView is print_job_status's own template data: a Job row,
// plus — only while a Layers-mode Job is "awaiting-next-pass" — the layer
// that just finished and the layer about to run next, both resolved to
// their label text (axicontrol-layers-pause-ui). Kept separate from
// jobRowView itself so /jobs's plain job_row rendering, and the
// general-purpose loadJobRow callers, never pay for the layer-label lookup
// this needs; both fields are nil for every other status.
type printJobStatusView struct {
	jobRowView
	FinishedLayer *layerView
	NextLayer     *layerView
}

// Done reports how many Passes have completed so far. PassNumber is the
// active Pass's own 1-indexed position, so the Passes before it are exactly
// the ones already complete — axicontrol-layers-pause-ui's "N of M done".
func (v printJobStatusView) Done() int {
	return v.PassNumber - 1
}

// NextLayerPreviewSrc is the upcoming layer's isolated-preview <img> source
// (svg.IsolateLayer, ADR-0010's <img>-only preview constraint), reusing the
// endpoint Single Layer mode's own live preview uses. Empty when there's no
// next layer to preview — the template's own {{if .NextLayer}} guard means
// this shouldn't be called otherwise, but a nil NextLayer here dereferences
// nothing rather than panicking, matching PreviewSrc's own defensiveness.
func (v printJobStatusView) NextLayerPreviewSrc() string {
	if v.NextLayer == nil {
		return ""
	}
	return fmt.Sprintf("/uploads/%d/layers/%d/content", v.FileID, v.NextLayer.Number)
}

// buildPrintJobStatusView assembles row's print-page status-card view,
// resolving the finished/next layer's label text only when row is a
// Layers-mode Job awaiting its next Pass (axicontrol-layers-pause-ui) — the
// one status where PrevLayerNumber/LayerNumber (jobs.go) both name an
// actual layer; every other status leaves FinishedLayer/NextLayer nil.
func (s *Server) buildPrintJobStatusView(ctx context.Context, row jobRowView) (printJobStatusView, error) {
	view := printJobStatusView{jobRowView: row}
	if row.Status != "awaiting-next-pass" {
		return view, nil
	}

	_, storageKey, err := s.loadFileRecord(ctx, row.FileID)
	if err != nil {
		return printJobStatusView{}, err
	}
	layers, err := s.discoverLayersForFile(ctx, storageKey)
	if err != nil {
		return printJobStatusView{}, err
	}
	views := buildLayerViews(layers)
	view.FinishedLayer = findLayerViewPtr(views, row.PrevLayerNumber)
	view.NextLayer = findLayerViewPtr(views, row.LayerNumber)
	return view, nil
}

// findLayerViewPtr finds number's entry in views (see buildLayerViews),
// returning nil if number itself is nil (no layer to look up) or isn't
// among the Upload's own discovered Layers — the latter shouldn't happen (a
// Pass's own layer_number always came from DiscoverLayers in the first
// place), but a caller gets nil rather than a fabricated label either way.
func findLayerViewPtr(views []layerView, number *int) *layerView {
	if number == nil {
		return nil
	}
	for _, v := range views {
		if v.Number == *number {
			return &v
		}
	}
	return nil
}

// printJobTargetID is the print page's own status card's DOM id for jobID
// (see print_job_status's "print-job-{{.ID}}") — the one Go-side place that
// spells it out, so renderJobActionResult's HX-Target comparison (jobs.go)
// can't drift from the template's own id without both call sites changing
// together.
func printJobTargetID(jobID int64) string {
	return fmt.Sprintf("print-job-%d", jobID)
}

// renderPrintJobStatus builds row's print-page status view (see
// buildPrintJobStatusView) and renders it as the print_job_status fragment,
// writing a 500 itself if the build fails — the common tail shared by
// handleShowJobPrintStatus and renderJobActionResult's print-page branch
// (jobs.go).
func (s *Server) renderPrintJobStatus(w http.ResponseWriter, r *http.Request, row jobRowView) {
	view, err := s.buildPrintJobStatusView(r.Context(), row)
	if err != nil {
		s.logger.Error("build print job status failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.renderFragment(w, http.StatusOK, "print_job_status", view)
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
// its currently in-progress Job if any (see loadInProgressJobForFile), and
// the device-busy Job belonging to a different Upload, if any (see
// loadBusyJobExcluding, axicontrol-device-busy-visibility).
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
	var jobView *printJobStatusView
	if job != nil {
		v, err := s.buildPrintJobStatusView(r.Context(), *job)
		if err != nil {
			return printPageView{}, err
		}
		jobView = &v
	}

	busyJob, err := s.loadBusyJobExcluding(r.Context(), upload.ID)
	if err != nil {
		return printPageView{}, err
	}

	return printPageView{
		Upload:    upload,
		Layers:    buildLayerViews(layers),
		Presets:   presets,
		Job:       jobView,
		BusyJob:   busyJob,
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
	s.renderPrintJobStatus(w, r, v)
}

// handleUploadBusyStatus renders upload's device-busy banner fragment (see
// print_device_busy), the poll target that fragment's own hx-trigger hits —
// it re-derives the AxiDraw's device-busy Job on every poll so the banner
// picks up a newly started Job elsewhere, updates as that Job progresses,
// and clears once it finishes, all without the viewer reloading
// (axicontrol-device-busy-visibility).
func (s *Server) handleUploadBusyStatus(w http.ResponseWriter, r *http.Request) {
	id, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	upload, ok := s.loadUploadOrNotFound(w, r, id)
	if !ok {
		return
	}

	busyJob, err := s.loadBusyJobExcluding(r.Context(), upload.ID)
	if err != nil {
		s.logger.Error("load busy job failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.renderFragment(w, http.StatusOK, "print_device_busy", printPageView{Upload: upload, BusyJob: busyJob})
}
