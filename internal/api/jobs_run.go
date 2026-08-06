package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// tryStartPrinting acquires the single-run lock: axicli is a subprocess
// talking to one physical, node-pinned AxiDraw (ADR-0001), so only one
// invocation may run at a time. Returns false if a Pass is already running.
func (s *Server) tryStartPrinting() bool {
	s.printMu.Lock()
	defer s.printMu.Unlock()
	if s.printing {
		return false
	}
	s.printing = true
	return true
}

func (s *Server) finishPrinting() {
	s.printMu.Lock()
	s.printing = false
	s.printMu.Unlock()
}

// executePass runs in its own goroutine, outside any HTTP request's
// lifetime, so it uses context.Background() rather than a request context
// that would be canceled as soon as the triggering handler returns.
func (s *Server) executePass(passID int64) {
	defer s.finishPrinting()
	ctx := context.Background()

	var jobID, fileID, presetID int64
	var overridesJSON string
	row := s.db.QueryRowContext(ctx, `SELECT p.job_id, j.file_id, p.preset_id, p.overrides
		FROM passes p JOIN jobs j ON j.id = p.job_id WHERE p.id = ?`, passID)
	if err := row.Scan(&jobID, &fileID, &presetID, &overridesJSON); err != nil {
		s.logger.Error("load pass for execution failed", "pass_id", passID, "error", err)
		return
	}

	if err := s.setPassStatus(ctx, passID, "running", ""); err != nil {
		s.logger.Error("mark pass running failed", "pass_id", passID, "error", err)
		return
	}

	output, err := s.runPass(ctx, fileID, presetID, overridesJSON)
	if err != nil {
		s.logger.Error("pass failed", "pass_id", passID, "error", err)
		if setErr := s.setPassStatus(ctx, passID, "failed", err.Error()); setErr != nil {
			s.logger.Error("mark pass failed failed", "pass_id", passID, "error", setErr)
		}
		return
	}

	if err := s.setPassStatus(ctx, passID, "complete", output); err != nil {
		s.logger.Error("mark pass complete failed", "pass_id", passID, "error", err)
	}
}

// runPass resolves Device Config + Preset + Pass-level overrides into a
// single config (ADR-0003) and invokes axicli against the file's sanitized
// SVG, returning its combined output on success or an error wrapping that
// output on failure.
func (s *Server) runPass(ctx context.Context, fileID, presetID int64, overridesJSON string) (string, error) {
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

	args := buildAxicliArgs(localPath, resolved, device, s.devicePath)
	out, err := s.runAxicli(args...)
	if err != nil {
		return "", fmt.Errorf("%s: %w", string(out), err)
	}
	return string(out), nil
}

// buildAxicliArgs builds the axicli command line for a whole-mode plot of
// filePath, given a fully-resolved plot config (Preset + overrides already
// applied) and the singleton Device Config. See
// https://axidraw.com/doc/cli_api/ for the flag reference.
func buildAxicliArgs(filePath string, cfg presetView, device deviceConfigView, devicePath string) []string {
	args := []string{
		filePath,
		"--mode", "plot",
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
	}
	if cfg.ConstSpeed {
		args = append(args, "--const_speed")
	}
	if devicePath != "" {
		args = append(args, "--port", devicePath)
	}
	return args
}
