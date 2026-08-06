package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
)

type sysinfoResponse struct {
	Output string `json:"output"`
}

func (s *Server) handleSysinfo(w http.ResponseWriter, r *http.Request) {
	args := []string{"sysinfo"}
	if s.devicePath != "" {
		args = append(args, "--port", s.devicePath)
	}

	out, err := s.runAxicli(r.Context(), args...)
	if err != nil {
		s.logger.Error("axicli sysinfo failed", "error", err, "output", string(out))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("axicli sysinfo: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, sysinfoResponse{Output: string(out)})
}

// runAxicliCmd shells out to the axicli binary. It's the production
// implementation of Server.runAxicli; tests substitute a fake. ctx
// cancellation sends SIGINT rather than the exec package's default
// SIGKILL, and (WaitDelay left unset) waits indefinitely for axicli to
// exit on its own rather than forcing it — pausing a plot means finishing
// the current line segment and writing a checkpoint before exiting, not an
// instant kill.
func runAxicliCmd(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "axicli", args...)
	cmd.Cancel = func() error {
		return cmd.Process.Signal(os.Interrupt)
	}
	return cmd.CombinedOutput()
}
