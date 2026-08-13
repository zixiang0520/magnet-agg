package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"magnet-agg/internal/cfg"
)

func (s *Server) authenticated(r *http.Request) bool {
	c := cfg.Get()
	if c.AccessPassword == "" {
		return true
	}
	ck, err := r.Cookie("sid")
	if err != nil || ck.Value == "" {
		return false
	}
	s.mu.RLock()
	tok := s.sessionToken
	s.mu.RUnlock()
	return tok != "" && subtle.ConstantTimeCompare([]byte(ck.Value), []byte(tok)) == 1
}

func (s *Server) startSession(w http.ResponseWriter) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	s.mu.Lock()
	s.sessionToken = tok
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name: "sid", Value: tok, Path: "/", HttpOnly: true,
		SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
	})
}

func (s *Server) clearSession(w http.ResponseWriter) {
	s.mu.Lock()
	s.sessionToken = ""
	s.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: "", Path: "/", MaxAge: -1})
}

func (s *Server) requireAuth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticated(r) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录或会话已过期"})
			return
		}
		h(w, r)
	}
}

func (s *Server) uiSession(w http.ResponseWriter, r *http.Request) {
	c := cfg.Get()
	writeJSON(w, 200, map[string]any{
		"ok":                  true,
		"logged_in":           s.authenticated(r),
		"has_access_password": c.AccessPassword != "",
		"setup_needed":        c.AccessPassword == "",
	})
}

func (s *Server) uiLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	c := cfg.Get()
	if c.AccessPassword == "" {
		writeJSON(w, 200, map[string]any{"ok": true, "setup_needed": true})
		return
	}
	if subtle.ConstantTimeCompare([]byte(body.Password), []byte(c.AccessPassword)) != 1 {
		writeJSON(w, 401, map[string]string{"error": "密码错误"})
		return
	}
	s.startSession(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) uiLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	s.clearSession(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) uiSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	c := cfg.Get()
	if c.AccessPassword != "" && !s.authenticated(r) {
		writeJSON(w, 403, map[string]string{"error": "已初始化，请先登录后再改密码"})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	body.Password = strings.TrimSpace(body.Password)
	if len(body.Password) < 4 {
		writeJSON(w, 400, map[string]string{"error": "密码至少 4 位"})
		return
	}
	c.AccessPassword = body.Password
	if err := cfg.Save(s.cfgPath, c); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.startSession(w)
	writeJSON(w, 200, map[string]any{"ok": true})
}

func publicPath(p string) bool {
	if p == "/login.html" || p == "/login.js" || p == "/style.css" || p == "/favicon.ico" {
		return true
	}
	if strings.HasPrefix(p, "/api/ui/") || p == "/api/health" {
		return true
	}
	return false
}

func (s *Server) gate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || publicPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if s.authenticated(r) {
			next.ServeHTTP(w, r)
			return
		}
		c := cfg.Get()
		if c.AccessPassword == "" && !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "未登录或会话已过期"})
			return
		}
		http.Redirect(w, r, "/login.html", http.StatusFound)
	})
}
