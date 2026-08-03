package api

import (
	"fmt"
	"net/http"
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

	out, err := s.runAxicli(args...)
	if err != nil {
		s.logger.Error("axicli sysinfo failed", "error", err, "output", string(out))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("axicli sysinfo: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, sysinfoResponse{Output: string(out)})
}

// runAxicliCmd shells out to the axicli binary. It's the production
// implementation of Server.runAxicli; tests substitute a fake.
func runAxicliCmd(args ...string) ([]byte, error) {
	return exec.Command("axicli", args...).CombinedOutput()
}
