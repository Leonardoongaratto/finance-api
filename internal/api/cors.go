package api

import "net/http"

const (
	corsMethods = "GET, POST, PUT, DELETE"
	corsHeaders = "Content-Type"
	corsMaxAge  = "600"
)

func originSet(origins []string) map[string]struct{} {
	set := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		if origin != "*" {
			set[origin] = struct{}{}
		}
	}
	return set
}

// cors lets the browser app, served from another origin, call the API. Only origins listed in the
// configuration are echoed back (never a wildcard); any other origin gets no CORS headers at all.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if _, allowed := s.corsOrigins[origin]; !allowed || origin == "" {
			next.ServeHTTP(w, r)
			return
		}

		header := w.Header()
		header.Add("Vary", "Origin")
		header.Set("Access-Control-Allow-Origin", origin)
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			header.Set("Access-Control-Allow-Methods", corsMethods)
			header.Set("Access-Control-Allow-Headers", corsHeaders)
			header.Set("Access-Control-Max-Age", corsMaxAge)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
