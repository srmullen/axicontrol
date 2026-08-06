package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// testingView is the hardware self-test/jog panel's current state: the
// carriage position axicontrol has tracked in memory since the last home or
// process start (ADR-0004), plus the outcome of whichever action was last
// triggered.
type testingView struct {
	CarriageX float64
	CarriageY float64
	Output    string
	Error     string
}

// modeArgs builds a bare-mode axicli invocation (cycle/toggle/align — the
// "utility" modes that take no file and no manual_cmd).
func modeArgs(mode, devicePath string) []string {
	args := []string{"--mode", mode}
	if devicePath != "" {
		args = append(args, "--port", devicePath)
	}
	return args
}

// manualCmdArgs builds a `--mode manual --manual_cmd <cmd>` invocation, with
// extra appended before --port (e.g. walk_x/walk_y's --dist value).
func manualCmdArgs(cmd, devicePath string, extra ...string) []string {
	args := []string{"--mode", "manual", "--manual_cmd", cmd}
	args = append(args, extra...)
	if devicePath != "" {
		args = append(args, "--port", devicePath)
	}
	return args
}

func (s *Server) currentTestingView(output, errMsg string) testingView {
	s.posMu.Lock()
	x, y := s.carriageX, s.carriageY
	s.posMu.Unlock()
	return testingView{CarriageX: x, CarriageY: y, Output: output, Error: errMsg}
}

// renderTestingPanel re-renders the #testing-panel fragment with the given
// action outcome layered onto the current tracked position.
func (s *Server) renderTestingPanel(w http.ResponseWriter, status int, output, errMsg string) {
	s.renderFragment(w, status, "testing_panel", s.currentTestingView(output, errMsg))
}

func (s *Server) handleTestingPage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "testing_content", s.currentTestingView("", ""))
}

// runTestAction claims the device for the duration of a single self-test/jog
// command and shells args through axicli, rendering the testing panel with
// its output. It claims and releases the device the same way a Job does
// (tryClaimDevice/releaseDevice) rather than merely peeking at whether one
// is claimed — exactly one physical AxiDraw exists (ADR-0001), so a jog
// command and a Pass's own axicli subprocess must never run concurrently,
// not just never be allowed to *start* concurrently. ok reports whether the
// caller should go on to do anything else (e.g. update tracked position);
// when false, this has already written the HTTP response.
func (s *Server) runTestAction(w http.ResponseWriter, r *http.Request, args []string) (output string, ok bool) {
	if !s.tryClaimDevice() {
		s.renderTestingPanel(w, http.StatusOK, "", deviceBusyMessage)
		return "", false
	}
	defer s.releaseDevice(true)

	out, err := s.runAxicli(r.Context(), args...)
	if err != nil {
		s.logger.Error("axicli test action failed", "args", args, "error", err, "output", string(out))
		s.renderTestingPanel(w, http.StatusOK, string(out), err.Error())
		return string(out), false
	}
	return string(out), true
}

func (s *Server) handleTestSysinfo(w http.ResponseWriter, r *http.Request) {
	out, ok := s.runTestAction(w, r, sysinfoArgs(s.devicePath))
	if !ok {
		return
	}
	s.renderTestingPanel(w, http.StatusOK, out, "")
}

func (s *Server) handleTestCycle(w http.ResponseWriter, r *http.Request) {
	out, ok := s.runTestAction(w, r, modeArgs("cycle", s.devicePath))
	if !ok {
		return
	}
	s.renderTestingPanel(w, http.StatusOK, out, "")
}

func (s *Server) handleTestToggle(w http.ResponseWriter, r *http.Request) {
	out, ok := s.runTestAction(w, r, modeArgs("toggle", s.devicePath))
	if !ok {
		return
	}
	s.renderTestingPanel(w, http.StatusOK, out, "")
}

func (s *Server) handleTestAlign(w http.ResponseWriter, r *http.Request) {
	out, ok := s.runTestAction(w, r, modeArgs("align", s.devicePath))
	if !ok {
		return
	}
	s.renderTestingPanel(w, http.StatusOK, out, "")
}

// handleTestWalkHome walks the carriage back to wherever the motors were
// last enabled and, on success, zeros axicontrol's own tracked position to
// match — the one reliable reference point the hardware gives (ADR-0004).
func (s *Server) handleTestWalkHome(w http.ResponseWriter, r *http.Request) {
	out, ok := s.runTestAction(w, r, manualCmdArgs("walk_home", s.devicePath))
	if !ok {
		return
	}

	s.posMu.Lock()
	s.carriageX, s.carriageY = 0, 0
	s.posMu.Unlock()

	s.renderTestingPanel(w, http.StatusOK, out, "")
}

// formatDist formats a --dist value the way axicli expects: a plain decimal,
// not Go's default %v float formatting (which can emit scientific notation
// for some magnitudes).
func formatDist(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// handleTestMove translates a "go to (x, y)" request into the relative
// walk_x/walk_y deltas needed from axicontrol's own tracked position
// (ADR-0004) — the AxiDraw itself has no absolute-position concept. Each
// axis only moves (and only updates tracked position) if its delta is
// nonzero, and a failure on either axis stops before touching the other so
// tracked position never claims a move that didn't actually happen. The
// device is claimed for the whole two-axis sequence (see runTestAction),
// not per axis, so a Job can't interleave a Pass between the walk_x and
// walk_y calls.
func (s *Server) handleTestMove(w http.ResponseWriter, r *http.Request) {
	if !s.tryClaimDevice() {
		s.renderTestingPanel(w, http.StatusOK, "", deviceBusyMessage)
		return
	}
	defer s.releaseDevice(true)

	if err := r.ParseForm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	x, err := strconv.ParseFloat(r.FormValue("x"), 64)
	if err != nil {
		s.renderTestingPanel(w, http.StatusOK, "", "x must be a number")
		return
	}
	y, err := strconv.ParseFloat(r.FormValue("y"), 64)
	if err != nil {
		s.renderTestingPanel(w, http.StatusOK, "", "y must be a number")
		return
	}

	s.posMu.Lock()
	deltaX := x - s.carriageX
	deltaY := y - s.carriageY
	s.posMu.Unlock()

	var output strings.Builder

	if deltaX != 0 {
		out, err := s.runAxicli(r.Context(), manualCmdArgs("walk_x", s.devicePath, "--dist", formatDist(deltaX))...)
		output.Write(out)
		if err != nil {
			s.logger.Error("axicli walk_x failed", "error", err, "output", string(out))
			s.renderTestingPanel(w, http.StatusOK, output.String(), fmt.Sprintf("move failed: %v", err))
			return
		}
		s.posMu.Lock()
		s.carriageX = x
		s.posMu.Unlock()
	}

	if deltaY != 0 {
		out, err := s.runAxicli(r.Context(), manualCmdArgs("walk_y", s.devicePath, "--dist", formatDist(deltaY))...)
		output.Write(out)
		if err != nil {
			s.logger.Error("axicli walk_y failed", "error", err, "output", string(out))
			s.renderTestingPanel(w, http.StatusOK, output.String(), fmt.Sprintf("move failed: %v", err))
			return
		}
		s.posMu.Lock()
		s.carriageY = y
		s.posMu.Unlock()
	}

	s.renderTestingPanel(w, http.StatusOK, output.String(), "")
}
