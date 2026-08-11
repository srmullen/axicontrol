package api

import "net/http"

// handleUploadPrintPage renders the page scoped to a single Upload where
// it's previewed and (in later tickets) printed. This is the only place an
// Upload's contents are shown to the user now — the uploads list itself has
// no inline preview (ADR-0017).
func (s *Server) handleUploadPrintPage(w http.ResponseWriter, r *http.Request) {
	id, err := uploadIDFromPath(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid upload id")
		return
	}

	v, ok := s.loadUploadOrNotFound(w, r, id)
	if !ok {
		return
	}

	s.renderPage(w, "print_content", v)
}
