package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"magnet-agg/internal/plugin"
	"magnet-agg/internal/search"
)

type Server struct {
	eng    *search.Engine
	reg    *plugin.Registry
	webDir string
}

func New(eng *search.Engine, reg *plugin.Registry, webDir string) *Server {
	return &Server{eng: eng, reg: reg, webDir: webDir}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/search", s.search)
	mux.HandleFunc("/api/plugins", s.plugins)
	fs := http.FileServer(http.Dir(s.webDir))
	mux.Handle("/", fs)
	return withCORS(mux)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(204)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"ok": true, "service": "magnet-agg"})
}

func (s *Server) plugins(w http.ResponseWriter, r *http.Request) {
	names := []string{}
	if s.reg != nil {
		names = s.reg.Names()
	}
	writeJSON(w, 200, map[string]any{"plugins": names})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, 400, map[string]string{"error": "missing q"})
		return
	}
	res := s.eng.Search(r.Context(), q)
	writeJSON(w, 200, res)
}
