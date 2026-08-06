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

// runRegistration tracks the axicli subprocess invocation currently in
// flight for one Pass, so a pause/cancel HTTP handler (running on a
// different goroutine) can reach in and ask it to stop. intent records
// which status an interrupted run should land on — "paused" or
// "cancelled" — set by requestInterrupt at the moment it's requested, and
// read back by executePass once the (now-canceled) run actually returns.
type runRegistration struct {
	passID int64
	cancel context.CancelFunc
	intent string
}

// beginRun registers passID as the currently-running Pass and returns a
// context whose cancellation (via requestInterrupt) SIGINTs its axicli
// subprocess (see runAxicliCmd). Must be paired with endRun once the run
// completes.
func (s *Server) beginRun(passID int64) (context.Context, *runRegistration) {
	ctx, cancel := context.WithCancel(context.Background())
	reg := &runRegistration{passID: passID, cancel: cancel}
	s.runMu.Lock()
	s.currentRun = reg
	s.runMu.Unlock()
	return ctx, reg
}

// endRun clears reg's registration, but only if it's still the current one
// — a stale reg (from a run that's already ended) must not clobber a
// different, newer run's registration.
func (s *Server) endRun(reg *runRegistration) {
	s.runMu.Lock()
	if s.currentRun == reg {
		s.currentRun = nil
	}
	s.runMu.Unlock()
}

// requestInterrupt asks the axicli subprocess currently running passID to
// stop (SIGINT), recording intent as the Pass status executePass should
// land on once the subprocess actually exits — pausing isn't instantaneous,
// so the status transition happens asynchronously from this call. Returns
// false if passID isn't the Pass currently running (e.g. the button was
// clicked just as the Pass finished on its own), in which case the caller
// has nothing to interrupt.
func (s *Server) requestInterrupt(passID int64, intent string) bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	if s.currentRun == nil || s.currentRun.passID != passID {
		return false
	}
	s.currentRun.intent = intent
	s.currentRun.cancel()
	return true
}

// checkpointKey is the FileStore key axicli's -o output SVG is written to
// and read back from for a given Pass — see ADR-0008.
func checkpointKey(passID int64) string {
	return fmt.Sprintf("checkpoints/%d.svg", passID)
}

// deleteCheckpoint removes passID's checkpoint file, if any (delete-if-
// exists — a failed Pass may never have written one). Logs rather than
// propagates: a leftover checkpoint file is untidy but not worth failing an
// otherwise-successful status transition over.
func (s *Server) deleteCheckpoint(passID int64) {
	if err := s.files.Delete(checkpointKey(passID)); err != nil {
		s.logger.Error("delete checkpoint failed", "pass_id", passID, "error", err)
	}
}

// setPassStatusLogged is setPassStatus for executePass's terminal/paused
// transitions, which have no request to propagate a write failure to —
// just log it. Returns whether the write succeeded, for callers (like the
// "complete" transition) that still need to skip their own follow-on work
// on failure.
func (s *Server) setPassStatusLogged(ctx context.Context, passID int64, status, output string) bool {
	if err := s.setPassStatus(ctx, passID, status, output); err != nil {
		s.logger.Error("set pass status failed", "pass_id", passID, "status", status, "error", err)
		return false
	}
	return true
}

// tryStartPassRun atomically transitions passID from priorStatus to
// "running", the only point where two goroutines can contend for one
// Pass's status: a cancel request arriving between executePass reading
// priorStatus and writing "running" would otherwise be silently clobbered
// back to running (once a Pass is actually running, pause/cancel instead go
// through requestInterrupt, which is race-free by construction). Returns
// false, no error, if it lost that race — priorStatus no longer matches,
// so someone else (a synchronous cancel) already moved it on.
func (s *Server) tryStartPassRun(ctx context.Context, passID int64, priorStatus string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE passes SET status = 'running', output = '', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ? AND status = ?`,
		passID, priorStatus)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// tryCancelIfNotRunning atomically cancels passID only if it's still
// pending or paused — i.e. hasn't started running since the caller last
// checked its status. Returns false, no error, if it lost that race (the
// Pass is now running); the caller should fall back to requestInterrupt.
func (s *Server) tryCancelIfNotRunning(ctx context.Context, passID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE passes SET status = 'cancelled', updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ? AND status IN ('pending', 'paused')`,
		passID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// executePass runs in its own goroutine, outside any HTTP request's
// lifetime, so it uses context.Background() rather than a request context
// that would be canceled as soon as the triggering handler returns — except
// for the axicli invocation itself, which runs against beginRun's
// cancelable context so a pause/cancel request can SIGINT it.
func (s *Server) executePass(passID int64) {
	ctx := context.Background()

	var jobID, fileID, presetID int64
	var overridesJSON, priorStatus string
	var layerNumber sql.NullInt64
	row := s.db.QueryRowContext(ctx, `SELECT p.job_id, j.file_id, p.preset_id, p.overrides, p.layer_number, p.status
		FROM passes p JOIN jobs j ON j.id = p.job_id WHERE p.id = ?`, passID)
	if err := row.Scan(&jobID, &fileID, &presetID, &overridesJSON, &layerNumber, &priorStatus); err != nil {
		s.logger.Error("load pass for execution failed", "pass_id", passID, "error", err)
		s.releaseDevice(true)
		return
	}
	resume := priorStatus == "paused"

	started, err := s.tryStartPassRun(ctx, passID, priorStatus)
	if err != nil {
		s.logger.Error("mark pass running failed", "pass_id", passID, "error", err)
		s.releaseDevice(true)
		return
	}
	if !started {
		// Lost a race with a concurrent cancel (handleCancelJob's
		// synchronous pending/paused branch got there first) — nothing to
		// run; that handler already recorded "cancelled" and its own
		// checkpoint cleanup.
		s.releaseDevice(true)
		return
	}

	var layerNumberPtr *int
	if layerNumber.Valid {
		n := int(layerNumber.Int64)
		layerNumberPtr = &n
	}

	runCtx, reg := s.beginRun(passID)
	output, err := s.runPass(runCtx, passRun{
		passID:        passID,
		fileID:        fileID,
		presetID:      presetID,
		overridesJSON: overridesJSON,
		layerNumber:   layerNumberPtr,
		resume:        resume,
	})
	s.endRun(reg)

	if err != nil && reg.intent != "" {
		// An intentional pause or cancel, not a genuine failure: the
		// subprocess was SIGINTed via requestInterrupt, so its non-nil
		// error here reflects that request rather than a real problem.
		_ = s.setPassStatusLogged(ctx, passID, reg.intent, "")
		if reg.intent == "paused" {
			// Not terminal: the checkpoint just written is exactly what
			// resume needs, and the device stays claimed by this Job.
			s.releaseDevice(false)
			return
		}
		s.deleteCheckpoint(passID)
		s.releaseDevice(true)
		return
	}

	if err != nil {
		s.logger.Error("pass failed", "pass_id", passID, "error", err)
		_ = s.setPassStatusLogged(ctx, passID, "failed", err.Error())
		s.deleteCheckpoint(passID)
		// A failed Pass always ends the Job's claim on the device (matching
		// whole-mode's existing retry-reclaims-it behavior); a mid-sequence
		// failure isn't a safe "leave it mounted" state to defend the way
		// awaiting-next-pass is.
		s.releaseDevice(true)
		return
	}

	if !s.setPassStatusLogged(ctx, passID, "complete", output) {
		s.releaseDevice(true)
		return
	}
	s.deleteCheckpoint(passID)

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

// passRun identifies one Pass execution: either a fresh run (resume false)
// against fileID's source SVG, using presetID/overridesJSON/layerNumber to
// build its config and mode; or a resume (resume true) of passID's own
// checkpoint file via res_plot, which only needs presetID/overridesJSON
// (fileID and layerNumber are unused — the checkpoint already encodes
// which file and which layer(s) were in progress).
type passRun struct {
	passID        int64
	fileID        int64
	presetID      int64
	overridesJSON string
	layerNumber   *int
	resume        bool
}

// runPass resolves Device Config + Preset + Pass-level overrides into a
// single config (ADR-0003) and invokes axicli, returning its combined
// output on success or an error wrapping that output on failure. Either a
// fresh or a resumed run writes its progress to the same checkpoint key
// (ADR-0008), via -o, so a pause always has somewhere to write to —
// including a resumed run's own subsequent pause.
func (s *Server) runPass(ctx context.Context, run passRun) (string, error) {
	preset, err := s.loadPreset(ctx, run.presetID)
	if err != nil {
		return "", fmt.Errorf("load preset: %w", err)
	}

	device, err := s.loadDeviceConfig(ctx)
	if err != nil {
		return "", fmt.Errorf("load device config: %w", err)
	}

	var ov overrides
	if err := json.Unmarshal([]byte(run.overridesJSON), &ov); err != nil {
		return "", fmt.Errorf("parse overrides: %w", err)
	}
	resolved := ov.apply(preset)

	target := axicliTarget{resume: run.resume}
	var cleanup func()
	if run.resume {
		target.filePath, cleanup, err = s.files.LocalPath(checkpointKey(run.passID))
		if err != nil {
			return "", fmt.Errorf("load checkpoint: %w", err)
		}
	} else {
		target.layerNumber = run.layerNumber
		_, storageKey, ferr := s.loadFileRecord(ctx, run.fileID)
		if ferr != nil {
			return "", fmt.Errorf("load file: %w", ferr)
		}
		target.filePath, cleanup, err = s.files.LocalPath(storageKey)
		if err != nil {
			return "", fmt.Errorf("load file contents: %w", err)
		}
	}
	defer cleanup()

	checkpointPath, checkpointCleanup, err := s.files.LocalWritePath(checkpointKey(run.passID))
	if err != nil {
		return "", fmt.Errorf("prepare checkpoint path: %w", err)
	}
	defer checkpointCleanup()

	args := buildAxicliArgs(target, checkpointPath, resolved, device, s.devicePath)
	out, err := s.runAxicli(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("%s: %w", string(out), err)
	}
	return string(out), nil
}

// axicliTarget is what a single axicli invocation should plot: either
// filePath's source SVG fresh (layerNumber selecting layers mode, nil for
// whole-mode's plot-everything mode), or, if resume, filePath as a
// checkpoint file via res_plot (layerNumber is meaningless then — the
// checkpoint already encodes which layer(s) were in progress).
type axicliTarget struct {
	filePath    string
	layerNumber *int
	resume      bool
}

// buildAxicliArgs builds the axicli command line for target, given a
// fully-resolved plot config (Preset + overrides already applied) and the
// singleton Device Config. checkpointPath is always passed via -o
// (ADR-0008). See https://axidraw.com/doc/cli_api/ for the flag reference.
func buildAxicliArgs(target axicliTarget, checkpointPath string, cfg presetView, device deviceConfigView, devicePath string) []string {
	args := []string{target.filePath}
	switch {
	case target.resume:
		args = append(args, "--mode", "res_plot")
	case target.layerNumber != nil:
		args = append(args, "--mode", "layers", "--layer", strconv.Itoa(*target.layerNumber))
	default:
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
	args = append(args, "-o", checkpointPath)
	return args
}
