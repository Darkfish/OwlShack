package api

import (
	"net/http"
	"strconv"
	"strings"
)

const companionPathPrefix = "/api/companions/"

// resolveCompanionRef rewrites an "<id>" or "<id>-<slug>" companion segment to that companion's
// current name, so a link survives a rename. Every /api/companions/{name} handler resolves the name
// itself, so doing this once here covers all of them.
//
// A segment that is not an id reference is left alone, and so is one naming an id that no longer
// exists: name-keyed URLs predate this and must keep working for bookmarks and installed PWAs.
func (s *Server) resolveCompanionRef(r *http.Request) *http.Request {
	rest, found := strings.CutPrefix(r.URL.Path, companionPathPrefix)
	if !found {
		return r
	}
	seg, tail, hasTail := strings.Cut(rest, "/")
	id, isRef := parseCompanionRef(seg)
	if !isRef {
		return r
	}
	// A companion really named "7" keeps its own URL even when another companion has id 7.
	if _, err := s.store.Companions.IDByName(r.Context(), seg); err == nil {
		return r
	}
	c, err := s.store.Companions.Get(r.Context(), id)
	if err != nil || c == nil {
		return r
	}

	out := r.Clone(r.Context())
	out.URL.Path = companionPathPrefix + c.Name
	if hasTail {
		out.URL.Path += "/" + tail
	}
	// Cleared so EscapedPath re-encodes from the name we just substituted.
	out.URL.RawPath = ""
	return out
}

// parseCompanionRef reads the id out of "12" or "12-akl". The slug is decoration and is not checked
// against the current name: a stale slug in an old link must still resolve.
func parseCompanionRef(seg string) (int64, bool) {
	digits, _, _ := strings.Cut(seg, "-")
	if digits == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(digits, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
