package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
)

// dryRunView is the outcome of a plot dry-run: axicli's own report_time
// output on success, or an inline error to show next to the new-job form
// that triggered it.
type dryRunView struct {
	Output string
	Error  string
}

// handleDryRun previews an uploaded file's plot: same config resolution
// (Device Config + Preset + overrides) a real print would use (ADR-0003,
// see resolvePlotConfig), but with axicli's --preview and --report_time
// flags added so it simulates timing/geometry without moving the device or
// lowering the pen (ADR-0004). Unlike a real Job/Pass, this never claims the
// device (see tryClaimDevice) — --preview never talks to the AxiDraw at
// all, so it can safely run even while another Job is printing.
func (s *Server) handleDryRun(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	fileID, err := strconv.ParseInt(r.FormValue("file_id"), 10, 64)
	if err != nil {
		s.renderFragment(w, http.StatusOK, "dry_run_result", dryRunView{Error: "select a file"})
		return
	}
	presetID, err := strconv.ParseInt(r.FormValue("preset_id"), 10, 64)
	if err != nil {
		s.renderFragment(w, http.StatusOK, "dry_run_result", dryRunView{Error: "select a preset"})
		return
	}

	ov, err := parseOverridesForm(r)
	if err != nil {
		s.renderFragment(w, http.StatusOK, "dry_run_result", dryRunView{Error: err.Error()})
		return
	}

	_, storageKey, err := s.loadFileRecord(r.Context(), fileID)
	if errors.Is(err, sql.ErrNoRows) {
		s.renderFragment(w, http.StatusOK, "dry_run_result", dryRunView{Error: "selected file not found"})
		return
	} else if err != nil {
		s.logger.Error("load file for dry run failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resolved, device, err := s.resolvePlotConfig(r.Context(), presetID, ov)
	if errors.Is(err, sql.ErrNoRows) {
		s.renderFragment(w, http.StatusOK, "dry_run_result", dryRunView{Error: "selected preset not found"})
		return
	} else if err != nil {
		s.logger.Error("resolve dry run config failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	filePath, cleanup, err := s.files.LocalPath(storageKey)
	if err != nil {
		s.logger.Error("load file contents for dry run failed", "error", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer cleanup()

	// No checkpointPath (""): a dry-run never writes an output file, only
	// ever an in-memory simulation (buildAxicliArgs skips -o when empty).
	args := buildAxicliArgs(axicliTarget{filePath: filePath}, "", resolved, device, s.devicePath)
	args = append(args, "--preview", "--report_time")

	out, err := s.runAxicli(r.Context(), args...)
	if err != nil {
		s.logger.Error("axicli dry run failed", "error", err, "output", string(out))
		// axicli's own combined output usually explains a real device/plot
		// error better than the bare Go error, but a launch-level failure
		// (e.g. the binary itself missing) never produces any output — fall
		// back to err so the user never sees a blank message.
		msg := err.Error()
		if len(out) > 0 {
			msg = fmt.Sprintf("%s: %s", out, err)
		}
		s.renderFragment(w, http.StatusOK, "dry_run_result", dryRunView{Error: msg})
		return
	}

	s.renderFragment(w, http.StatusOK, "dry_run_result", dryRunView{Output: string(out)})
}
