package api

import (
	"net/http"
	"time"

	"github.com/meshcore-go/OwlShack/internal/buildinfo"
)

// handleHealth answers 200 whenever this process is alive, including when the radio is not. A
// monitor that cannot reach OwlShack at all already fails the request on its own, so spending the
// status code on a second opinion would only take the choice of what to alert on away from the
// operator. The body carries the facts to make that choice from.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	var info HealthInfo
	if b := s.backendRef(); b != nil {
		info = b.Health()
	} else {
		// Up, listening, nothing wired behind it yet — a real state during startup and after a
		// failed reload, and one a monitor should be able to see rather than read as healthy.
		info.Problems = []string{"no backend installed yet"}
	}

	if info.Problems == nil {
		info.Problems = []string{} // a JSON null would break $count(problems) in a monitor's query
	}
	info.Status = "ok"
	if len(info.Problems) > 0 {
		info.Status = "degraded"
	}
	info.Version = buildinfo.Version
	info.UptimeSecs = int64(time.Since(s.startedAt).Seconds())

	writeJSON(w, http.StatusOK, info)
}
