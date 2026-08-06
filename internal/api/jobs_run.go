package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
)

// tryClaimDevice claims the device for a brand-new Job submission or for
// retrying a failed one (both start from an unclaimed device — see
// releaseDevice). Returns false if another Job already claims it.
func (s *Server) tryClaimDevice() bool {
	s.printMu.Lock()
	defer s.printMu.Unlock()
	if s.deviceClaimed {
		return false
	}
	s.deviceClaimed = true
	s.passRunning = true
	return true
}

// tryStartNextPass starts the next Pass of a Job that already claims the
// device — layers mode's advance action. The device stays claimed across a
// layers-mode Job's awaiting-next-pass gap (see releaseDevice) so an
// unrelated Job can't interleave a Pass on the same still-mounted artwork
// between layers; this only guards against double-starting the same Pass
// (e.g. a double-clicked advance).
func (s *Server) tryStartNextPass() bool {
	s.printMu.Lock()
	defer s.printMu.Unlock()
	if s.passRunning {
		return false
	}
	s.passRunning = true
	return true
}

// releaseDevice marks the current Pass no longer running, and releases the
// Job's claim on the device entirely once jobDone — the Job has reached a
// terminal state (complete/failed/cancelled) rather than merely
// awaiting-next-pass.
func (s *Server) releaseDevice(jobDone bool) {
	s.printMu.Lock()
	s.passRunning = false
	if jobDone {
		s.deviceClaimed = false
	}
	s.printMu.Unlock()
}

// executePass runs in its own goroutine, outside any HTTP request's
// lifetime, so it uses context.Background() rather than a request context
// that would be canceled as soon as the triggering handler returns.
func (s *Server) executePass(passID int64) {
	ctx := context.Background()

	var jobID, fileID, presetID int64
	var overridesJSON string
	var layerNumber sql.NullInt64
	row := s.db.QueryRowContext(ctx, `SELECT p.job_id, j.file_id, p.preset_id, p.overrides, p.layer_number
		FROM passes p JOIN jobs j ON j.id = p.job_id WHERE p.id = ?`, passID)
	if err := row.Scan(&jobID, &fileID, &presetID, &overridesJSON, &layerNumber); err != nil {
		s.logger.Error("load pass for execution failed", "pass_id", passID, "error", err)
		s.releaseDevice(true)
		return
	}

	if err := s.setPassStatus(ctx, passID, "running", ""); err != nil {
		s.logger.Error("mark pass running failed", "pass_id", passID, "error", err)
		s.releaseDevice(true)
		return
	}

	var layerNumberPtr *int
	if layerNumber.Valid {
		n := int(layerNumber.Int64)
		layerNumberPtr = &n
	}

	output, err := s.runPass(ctx, fileID, presetID, overridesJSON, layerNumberPtr)
	if err != nil {
		s.logger.Error("pass failed", "pass_id", passID, "error", err)
		if setErr := s.setPassStatus(ctx, passID, "failed", err.Error()); setErr != nil {
			s.logger.Error("mark pass failed failed", "pass_id", passID, "error", setErr)
		}
		// A failed Pass always ends the Job's claim on the device (matching
		// whole-mode's existing retry-reclaims-it behavior); a mid-sequence
		// failure isn't a safe "leave it mounted" state to defend the way
		// awaiting-next-pass is.
		s.releaseDevice(true)
		return
	}

	if err := s.setPassStatus(ctx, passID, "complete", output); err != nil {
		s.logger.Error("mark pass complete failed", "pass_id", passID, "error", err)
		s.releaseDevice(true)
		return
	}

	jobDone, err := s.isJobDone(ctx, jobID)
	if err != nil {
		s.logger.Error("check job completion failed", "job_id", jobID, "error", err)
		jobDone = true // don't leave the device claimed forever if we can't tell
	}
	s.releaseDevice(jobDone)
}

// isJobDone reports whether jobID's derived status (see deriveJobStatus) is
// terminal — as opposed to awaiting-next-pass, which still holds the
// device's claim for this Job's remaining Passes.
func (s *Server) isJobDone(ctx context.Context, jobID int64) (bool, error) {
	passes, err := s.loadPassSummariesForJob(ctx, jobID)
	if err != nil {
		return false, err
	}
	switch deriveJobStatus(passes) {
	case "complete", "failed", "cancelled":
		return true, nil
	default:
		return false, nil
	}
}

// runPass resolves Device Config + Preset + Pass-level overrides into a
// single config (ADR-0003) and invokes axicli against the file's sanitized
// SVG, returning its combined output on success or an error wrapping that
// output on failure.
func (s *Server) runPass(ctx context.Context, fileID, presetID int64, overridesJSON string, layerNumber *int) (string, error) {
	_, storageKey, err := s.loadFileRecord(ctx, fileID)
	if err != nil {
		return "", fmt.Errorf("load file: %w", err)
	}

	preset, err := s.loadPreset(ctx, presetID)
	if err != nil {
		return "", fmt.Errorf("load preset: %w", err)
	}

	device, err := s.loadDeviceConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load device config: %w", err)
	}

	var ov overrides
	if err := json.Unmarshal([]byte(overridesJSON), &ov); err != nil {
		return "", fmt.Errorf("parse overrides: %w", err)
	}
	resolved := ov.apply(preset)

	localPath, cleanup, err := s.files.LocalPath(storageKey)
	if err != nil {
		return "", fmt.Errorf("load file contents: %w", err)
	}
	defer cleanup()

	args := buildAxicliArgs(localPath, resolved, device, s.devicePath, layerNumber)
	out, err := s.runAxicli(args...)
	if err != nil {
		return "", fmt.Errorf("%s: %w", string(out), err)
	}
	return string(out), nil
}

// buildAxicliArgs builds the axicli command line for a Pass against
// filePath, given a fully-resolved plot config (Preset + overrides already
// applied) and the singleton Device Config. layerNumber selects layers mode
// (only layers named with that numeric prefix plot, per AxiDraw's
// convention); nil means whole-mode's plot-everything mode. See
// https://axidraw.com/doc/cli_api/ for the flag reference.
func buildAxicliArgs(filePath string, cfg presetView, device deviceConfigView, devicePath string, layerNumber *int) []string {
	args := []string{filePath}
	if layerNumber != nil {
		args = append(args, "--mode", "layers", "--layer", strconv.Itoa(*layerNumber))
	} else {
		args = append(args, "--mode", "plot")
	}
	args = append(args,
		"--speed_pendown", strconv.Itoa(cfg.SpeedPendown),
		"--speed_penup", strconv.Itoa(cfg.SpeedPenup),
		"--accel", strconv.Itoa(cfg.Accel),
		"--pen_pos_down", strconv.Itoa(cfg.PenPosDown),
		"--pen_pos_up", strconv.Itoa(cfg.PenPosUp),
		"--pen_rate_lower", strconv.Itoa(cfg.PenRateLower),
		"--pen_rate_raise", strconv.Itoa(cfg.PenRateRaise),
		"--pen_delay_down", strconv.Itoa(cfg.PenDelayDown),
		"--pen_delay_up", strconv.Itoa(cfg.PenDelayUp),
		"--model", strconv.Itoa(device.Model),
		"--penlift", strconv.Itoa(device.Penlift),
	)
	if cfg.ConstSpeed {
		args = append(args, "--const_speed")
	}
	if devicePath != "" {
		args = append(args, "--port", devicePath)
	}
	return args
}
