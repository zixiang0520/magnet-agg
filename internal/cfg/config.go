package cfg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type PluginCfg struct {
	Enabled bool   `json:"enabled"`
	Base    string `json:"base,omitempty"`
	Proxy   string `json:"proxy,omitempty"`
}

type Config struct {
	Listen  string               `json:"listen"`
	DataDir string               `json:"data_dir"`
	Plugins map[string]PluginCfg `json:"plugins"`

	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	TokenFile    string `json:"token_file"`
	BaseDir      string `json:"base_dir"`

	TmdbAPIKey string `json:"tmdb_api_key"`
	TmdbProxy  string `json:"tmdb_proxy"`
	TmdbLang   string `json:"tmdb_language"`

	AIBaseURL string `json:"ai_base_url"`
	AIAPIKey  string `json:"ai_api_key"`
	AIModel   string `json:"ai_model"`
	AIProxy   string `json:"ai_proxy"`

	AccessPassword string `json:"access_password"`
}

var (
	mu   sync.RWMutex
	cur  *Config
	path string
)

func Defaults() *Config {
	return &Config{
		Listen:  ":8080",
		DataDir: "data",
		Plugins: map[string]PluginCfg{
			"6v520":        {Enabled: true, Base: "https://www.6v520.com"},
			"apibay":       {Enabled: true, Proxy: "http://192.168.1.100:20172"},
			"torrents-csv": {Enabled: true, Proxy: "http://192.168.1.100:20172"},
			"yts":          {Enabled: false, Proxy: "http://192.168.1.100:20172"},
		},
		TokenFile: "data/token.json",
		BaseDir:   "磁力聚合",
		TmdbProxy: "http://192.168.1.100:20172",
		TmdbLang:  "zh-CN",
		AIModel:   "gpt-4o-mini",
	}
}

func Load(p string) (*Config, error) {
	c := Defaults()
	path = p
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			_ = Save(p, c)
			mu.Lock()
			cur = c
			mu.Unlock()
			return c, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, err
	}
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	if c.TokenFile == "" {
		c.TokenFile = filepath.Join(c.DataDir, "token.json")
	}
	if c.BaseDir == "" {
		c.BaseDir = "磁力聚合"
	}
	if c.TmdbLang == "" {
		c.TmdbLang = "zh-CN"
	}
	if c.Plugins == nil {
		c.Plugins = Defaults().Plugins
	}
	mu.Lock()
	cur = c
	mu.Unlock()
	return c, nil
}

func Save(p string, c *Config) error {
	if p == "" {
		p = path
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		return err
	}
	mu.Lock()
	cp := *c
	if c.Plugins != nil {
		cp.Plugins = make(map[string]PluginCfg, len(c.Plugins))
		for k, v := range c.Plugins {
			cp.Plugins[k] = v
		}
	}
	cur = &cp
	path = p
	mu.Unlock()
	return nil
}

func Get() *Config {
	mu.RLock()
	defer mu.RUnlock()
	if cur == nil {
		return Defaults()
	}
	cp := *cur
	if cur.Plugins != nil {
		cp.Plugins = make(map[string]PluginCfg, len(cur.Plugins))
		for k, v := range cur.Plugins {
			cp.Plugins[k] = v
		}
	}
	return &cp
}

func Path() string { return path }

// Public 返回给前端的脱敏配置。
func Public(c *Config) map[string]any {
	if c == nil {
		c = Defaults()
	}
	return map[string]any{
		"listen":             c.Listen,
		"plugins":            c.Plugins,
		"client_id":          c.ClientID,
		"has_client_secret":  c.ClientSecret != "",
		"base_dir":           c.BaseDir,
		"has_tmdb_api_key":   c.TmdbAPIKey != "",
		"tmdb_proxy":         c.TmdbProxy,
		"tmdb_language":      c.TmdbLang,
		"ai_base_url":        c.AIBaseURL,
		"has_ai_api_key":      c.AIAPIKey != "",
		"ai_model":            c.AIModel,
		"ai_proxy":            c.AIProxy,
		"has_access_password": c.AccessPassword != "",
	}
}
