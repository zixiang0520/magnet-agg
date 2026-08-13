package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"magnet-agg/internal/cfg"
	"magnet-agg/internal/classify"
	"magnet-agg/internal/drive"
	"magnet-agg/internal/plugin"
	"magnet-agg/internal/search"
	"magnet-agg/internal/tmdb"
)

type Server struct {
	eng     *search.Engine
	drv     *drive.Client
	webDir  string
	cfgPath string

	mu           sync.RWMutex
	sessionToken string
}

func New(eng *search.Engine, drv *drive.Client, webDir, cfgPath string) *Server {
	return &Server{eng: eng, drv: drv, webDir: webDir, cfgPath: cfgPath}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/ui/session", s.uiSession)
	mux.HandleFunc("/api/ui/login", s.uiLogin)
	mux.HandleFunc("/api/ui/logout", s.uiLogout)
	mux.HandleFunc("/api/ui/setup", s.uiSetup)
	mux.HandleFunc("/api/search", s.search)
	mux.HandleFunc("/api/plugins", s.plugins)
	mux.HandleFunc("/api/settings", s.settings)
	mux.HandleFunc("/api/plugins/test", s.testPlugin)
	mux.HandleFunc("/api/tmdb/test", s.testTMDB)
	mux.HandleFunc("/api/ai/test", s.testAI)
	mux.HandleFunc("/api/tmdb/discover", s.tmdbDiscover)
	mux.HandleFunc("/api/tmdb/poster", s.tmdbPoster)
	mux.HandleFunc("/api/2dland/status", s.landStatus)
	mux.HandleFunc("/api/2dland/login", s.landLogin)
	mux.HandleFunc("/api/2dland/poll", s.landPoll)
	mux.HandleFunc("/api/2dland/logout", s.landLogout)
	mux.HandleFunc("/api/push", s.push)
	mux.HandleFunc("/api/tasks", s.tasks)
	mux.HandleFunc("/api/tasks/delete", s.deleteTask)
	mux.HandleFunc("/api/tasks/organize", s.organize)
	fs := http.FileServer(http.Dir(s.webDir))
	mux.Handle("/", fs)
	return withCORS(s.gate(mux))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
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

func decodeJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(v)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	c := cfg.Get()
	writeJSON(w, 200, map[string]any{
		"ok": true, "service": "magnet-agg",
		"plugins": s.eng.Registry().Names(),
		"logged_in": s.drv != nil && s.drv.LoggedIn(),
		"has_2dland": c.ClientID != "" && c.ClientSecret != "",
		"auth":       s.authenticated(r),
	})
}

func (s *Server) plugins(w http.ResponseWriter, r *http.Request) {
	c := cfg.Get()
	type row struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Base    string `json:"base,omitempty"`
		Proxy   string `json:"proxy,omitempty"`
		Live    bool   `json:"live"`
	}
	live := map[string]bool{}
	for _, n := range s.eng.Registry().Names() {
		live[n] = true
	}
	out := make([]row, 0, len(plugin.KnownPlugins))
	for _, name := range plugin.KnownPlugins {
		pc := c.Plugins[name]
		out = append(out, row{Name: name, Enabled: pc.Enabled, Base: pc.Base, Proxy: pc.Proxy, Live: live[name]})
	}
	writeJSON(w, 200, map[string]any{"plugins": out})
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, 400, map[string]string{"error": "missing q"})
		return
	}
	writeJSON(w, 200, s.eng.Search(r.Context(), q))
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, 200, cfg.Public(cfg.Get()))
	case http.MethodPost, http.MethodPut:
		s.saveSettings(w, r)
	default:
		w.WriteHeader(405)
	}
}

func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Plugins      map[string]cfg.PluginCfg `json:"plugins"`
		ClientID     *string                  `json:"client_id"`
		ClientSecret *string                  `json:"client_secret"`
		BaseDir      *string                  `json:"base_dir"`
		TmdbAPIKey   *string                  `json:"tmdb_api_key"`
		TmdbProxy    *string                  `json:"tmdb_proxy"`
		TmdbLang     *string                  `json:"tmdb_language"`
		AIBaseURL    *string                  `json:"ai_base_url"`
		AIAPIKey     *string                  `json:"ai_api_key"`
		AIModel      *string                  `json:"ai_model"`
		AIProxy      *string                  `json:"ai_proxy"`
		AccessPassword *string                `json:"access_password"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	c := cfg.Get()
	credsChanged := false
	if body.Plugins != nil {
		if c.Plugins == nil {
			c.Plugins = map[string]cfg.PluginCfg{}
		}
		for k, v := range body.Plugins {
			c.Plugins[k] = v
		}
	}
	if body.ClientID != nil && *body.ClientID != c.ClientID {
		c.ClientID = *body.ClientID
		credsChanged = true
	}
	if body.ClientSecret != nil && *body.ClientSecret != "" {
		c.ClientSecret = *body.ClientSecret
		credsChanged = true
	}
	if body.BaseDir != nil && *body.BaseDir != "" {
		c.BaseDir = *body.BaseDir
	}
	if body.TmdbAPIKey != nil && *body.TmdbAPIKey != "" {
		c.TmdbAPIKey = *body.TmdbAPIKey
	}
	if body.TmdbProxy != nil {
		c.TmdbProxy = *body.TmdbProxy
	}
	if body.TmdbLang != nil && *body.TmdbLang != "" {
		c.TmdbLang = *body.TmdbLang
	}
	if body.AIBaseURL != nil {
		c.AIBaseURL = *body.AIBaseURL
	}
	if body.AIAPIKey != nil && *body.AIAPIKey != "" {
		c.AIAPIKey = *body.AIAPIKey
	}
	if body.AIModel != nil && *body.AIModel != "" {
		c.AIModel = *body.AIModel
	}
	if body.AIProxy != nil {
		c.AIProxy = *body.AIProxy
	}
	if body.AccessPassword != nil && strings.TrimSpace(*body.AccessPassword) != "" {
		c.AccessPassword = strings.TrimSpace(*body.AccessPassword)
	}
	if err := cfg.Save(s.cfgPath, c); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.rebuildPlugins(c)
	if s.drv != nil {
		if credsChanged && c.ClientID != "" && c.ClientSecret != "" {
			s.drv.UpdateCredentials(c.ClientID, c.ClientSecret)
		}
		s.drv.Reload(c)
	}
	writeJSON(w, 200, cfg.Public(c))
}

func (s *Server) rebuildPlugins(c *cfg.Config) {
	var specs []plugin.PluginSpec
	for _, name := range plugin.KnownPlugins {
		pc := c.Plugins[name]
		specs = append(specs, plugin.PluginSpec{Name: name, Enabled: pc.Enabled, Base: pc.Base, Proxy: pc.Proxy})
	}
	s.eng.SetRegistry(plugin.Build(specs))
}

func (s *Server) testPlugin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var body struct {
		Name  string `json:"name"`
		Query string `json:"q"`
		Base  string `json:"base"`
		Proxy string `json:"proxy"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if body.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "missing name"})
		return
	}
	c := cfg.Get()
	pc := c.Plugins[body.Name]
	if body.Base != "" {
		pc.Base = body.Base
	}
	if body.Proxy != "" {
		pc.Proxy = body.Proxy
	}
	ctx, cancel := r.Context(), func() {}
	_ = cancel
	start := time.Now()
	n, err := plugin.Probe(ctx, plugin.PluginSpec{Name: body.Name, Enabled: true, Base: pc.Base, Proxy: pc.Proxy}, body.Query)
	out := map[string]any{"name": body.Name, "ok": err == nil, "hits": n, "took_ms": time.Since(start).Milliseconds()}
	if err != nil {
		out["error"] = err.Error()
	}
	writeJSON(w, 200, out)
}

func (s *Server) testTMDB(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var body struct {
		APIKey string `json:"tmdb_api_key"`
		Proxy  string `json:"tmdb_proxy"`
		Lang   string `json:"tmdb_language"`
		Query  string `json:"q"`
	}
	if err := decodeJSON(r, &body); err != nil && err != io.EOF {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	c := cfg.Get()
	key := strings.TrimSpace(body.APIKey)
	if key == "" {
		key = c.TmdbAPIKey
	}
	proxy := strings.TrimSpace(body.Proxy)
	if proxy == "" {
		proxy = c.TmdbProxy
	}
	lang := strings.TrimSpace(body.Lang)
	if lang == "" {
		lang = c.TmdbLang
	}
	if key == "" {
		writeJSON(w, 400, map[string]string{"error": "未配置 TMDB API Key"})
		return
	}
	q := strings.TrimSpace(body.Query)
	if q == "" {
		q = "Inception"
	}
	cli := tmdb.New(key, proxy, lang)
	start := time.Now()
	hit, err := cli.Search(r.Context(), q, "movie")
	out := map[string]any{"ok": err == nil && hit != nil, "proxy": proxy, "took_ms": time.Since(start).Milliseconds()}
	if err != nil {
		out["error"] = err.Error()
	} else if hit != nil {
		out["title"] = hit.Title
		out["date"] = hit.Date
	} else {
		out["error"] = "无结果"
	}
	writeJSON(w, 200, out)
}

func (s *Server) testAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var body struct {
		Base  string `json:"ai_base_url"`
		Key   string `json:"ai_api_key"`
		Model string `json:"ai_model"`
		Proxy string `json:"ai_proxy"`
	}
	if err := decodeJSON(r, &body); err != nil && err != io.EOF {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	c := cfg.Get()
	base := strings.TrimSpace(body.Base)
	if base == "" {
		base = c.AIBaseURL
	}
	key := strings.TrimSpace(body.Key)
	if key == "" {
		key = c.AIAPIKey
	}
	model := strings.TrimSpace(body.Model)
	if model == "" {
		model = c.AIModel
	}
	proxy := strings.TrimSpace(body.Proxy)
	if proxy == "" {
		proxy = c.AIProxy
	}
	if base == "" || key == "" {
		writeJSON(w, 400, map[string]string{"error": "未配置 AI Base URL / API Key"})
		return
	}
	cli := classify.New(base, key, model, proxy)
	start := time.Now()
	reply, err := cli.Ping(r.Context())
	out := map[string]any{
		"ok":      err == nil,
		"model":   model,
		"base":    base,
		"proxy":   proxy,
		"took_ms": time.Since(start).Milliseconds(),
	}
	if err != nil {
		out["error"] = err.Error()
	} else {
		out["reply"] = truncateRunes(reply, 40)
	}
	writeJSON(w, 200, out)
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func tmdbClientFromCfg() (*tmdb.Client, error) {
	c := cfg.Get()
	if strings.TrimSpace(c.TmdbAPIKey) == "" {
		return nil, fmt.Errorf("未配置 TMDB API Key")
	}
	return tmdb.New(c.TmdbAPIKey, c.TmdbProxy, c.TmdbLang), nil
}

func (s *Server) tmdbDiscover(w http.ResponseWriter, r *http.Request) {
	cli, err := tmdbClientFromCfg()
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("type"))
	if kind != "tv" {
		kind = "movie"
	}
	items, err := cli.Discover(r.Context(), kind, 12)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"type": kind, "items": items})
}

func (s *Server) tmdbPoster(w http.ResponseWriter, r *http.Request) {
	cli, err := tmdbClientFromCfg()
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	path := strings.TrimSpace(r.URL.Query().Get("path"))
	resp, err := cli.FetchPoster(r.Context(), path)
	if err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		http.Error(w, "poster fetch failed", resp.StatusCode)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "image/jpeg"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 4<<20))
}

func (s *Server) landStatus(w http.ResponseWriter, r *http.Request) {
	c := cfg.Get()
	writeJSON(w, 200, map[string]any{
		"logged_in":         s.drv != nil && s.drv.LoggedIn(),
		"has_credentials":   c.ClientID != "" && c.ClientSecret != "",
		"client_id":         c.ClientID,
		"base_dir":          c.BaseDir,
		"has_tmdb":          c.TmdbAPIKey != "",
		"has_ai":            c.AIAPIKey != "" && c.AIBaseURL != "",
	})
}

func (s *Server) landLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if s.drv == nil || !s.drv.HasCredentials() {
		writeJSON(w, 400, map[string]string{"error": "请先在后台填写 2dland client_id / client_secret"})
		return
	}
	res, err := s.drv.StartLogin(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) landPoll(w http.ResponseWriter, r *http.Request) {
	if s.drv == nil {
		writeJSON(w, 400, map[string]string{"error": "2dland 未初始化"})
		return
	}
	res, err := s.drv.PollLogin(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) landLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if s.drv != nil {
		s.drv.Logout()
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) push(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	if s.drv == nil || !s.drv.LoggedIn() {
		writeJSON(w, 400, map[string]string{"error": "尚未绑定 2dland，请先到后台登录"})
		return
	}
	var body struct {
		Magnets []drive.PushItem `json:"magnets"`
		Query   string           `json:"query"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if len(body.Magnets) == 0 {
		writeJSON(w, 400, map[string]string{"error": "未选择磁力链"})
		return
	}
	if body.Query != "" {
		for i := range body.Magnets {
			if body.Magnets[i].Query == "" {
				body.Magnets[i].Query = body.Query
			}
			if body.Magnets[i].Title == "" {
				body.Magnets[i].Title = body.Query
			}
		}
	}
	res, err := s.drv.Push(r.Context(), body.Magnets)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}

func (s *Server) tasks(w http.ResponseWriter, r *http.Request) {
	if s.drv == nil || !s.drv.LoggedIn() {
		writeJSON(w, 400, map[string]string{"error": "尚未绑定 2dland"})
		return
	}
	tasks, err := s.drv.ListTasks(r.Context())
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, tasks)
}

func (s *Server) deleteTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var body struct {
		Identities  []string `json:"identities"`
		DeleteFiles bool     `json:"delete_files"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if err := s.drv.DeleteTask(r.Context(), body.Identities, body.DeleteFiles); err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) organize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	var body struct {
		SavePath string `json:"save_path"`
	}
	if err := decodeJSON(r, &body); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if body.SavePath == "" {
		writeJSON(w, 400, map[string]string{"error": "缺少 save_path"})
		return
	}
	res, err := s.drv.OrganizeTask(r.Context(), body.SavePath)
	if err != nil {
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, res)
}
