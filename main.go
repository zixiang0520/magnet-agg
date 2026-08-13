package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"magnet-agg/internal/cfg"
	"magnet-agg/internal/drive"
	"magnet-agg/internal/plugin"
	"magnet-agg/internal/search"
	"magnet-agg/internal/server"
)

func main() {
	cfgPath := env("CONFIG", "data/config.json")
	c, err := cfg.Load(cfgPath)
	if err != nil {
		log.Fatal(err)
	}
	if v := env("LISTEN", ""); v != "" {
		c.Listen = v
	}
	if err := os.MkdirAll(filepath.Dir(c.TokenFile), 0o755); err != nil {
		log.Fatal(err)
	}

	var specs []plugin.PluginSpec
	for _, name := range plugin.KnownPlugins {
		pc := c.Plugins[name]
		specs = append(specs, plugin.PluginSpec{Name: name, Enabled: pc.Enabled, Base: pc.Base, Proxy: pc.Proxy})
	}
	reg := plugin.Build(specs)
	eng := search.NewEngine(reg, 45*time.Second)
	drv := drive.New(c)
	srv := server.New(eng, drv, "web", cfgPath)

	log.Printf("magnet-agg listening on %s plugins=%v logged_in=%v", c.Listen, reg.Names(), drv.LoggedIn())
	if err := http.ListenAndServe(c.Listen, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
