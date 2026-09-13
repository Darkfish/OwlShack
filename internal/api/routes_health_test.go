package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubBackend embeds the interface so only Health needs a body; anything else the handler touched
// would panic loudly rather than pass silently.
type stubBackend struct {
	Backend
	info HealthInfo
}

func (s stubBackend) Health() HealthInfo { return s.info }

func getHealth(t *testing.T, s *Server) (*http.Response, HealthInfo) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	var info HealthInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body.String(), err)
	}
	return rec.Result(), info
}

// The status code is deliberately not a second opinion: a monitor that cannot reach OwlShack fails
// the request already, so spending it here would take the choice of what to alert on away.
func TestHealth_AlwaysAnswers200(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		backend  Backend
		wantStat string
	}{
		{"no backend installed", nil, "degraded"},
		{"healthy", stubBackend{}, "ok"},
		{"radio down", stubBackend{info: HealthInfo{Problems: []string{"radio: modem not connected"}}}, "degraded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewServer(nil, nil, nil)
			if tc.backend != nil {
				s.SetBackend(tc.backend)
			}
			resp, info := getHealth(t, s)
			if resp.StatusCode != http.StatusOK {
				t.Errorf("status = %d, want 200 even when unhealthy", resp.StatusCode)
			}
			if info.Status != tc.wantStat {
				t.Errorf("status = %q, want %q", info.Status, tc.wantStat)
			}
		})
	}
}

// A JSON null here would break $count(problems) in a monitor's query, which is the simplest thing
// an operator can write against this endpoint.
func TestHealth_ProblemsIsNeverNull(t *testing.T) {
	t.Parallel()
	s := NewServer(nil, nil, nil)
	s.SetBackend(stubBackend{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["problems"]); got != "[]" {
		t.Errorf("problems rendered as %s, want []", got)
	}
}

func TestHealth_StampsVersionAndUptime(t *testing.T) {
	t.Parallel()
	s := NewServer(nil, nil, nil)
	s.SetBackend(stubBackend{})
	_, info := getHealth(t, s)

	if info.Version == "" {
		t.Error("version is empty; a monitor cannot tell which build answered")
	}
	if info.UptimeSecs < 0 {
		t.Errorf("uptimeSecs = %d", info.UptimeSecs)
	}
}
