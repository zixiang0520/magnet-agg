package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"magnet-agg/internal/plugin"
	"magnet-agg/internal/search"
	"magnet-agg/internal/server"
)

func main() {
	addr := env("LISTEN", ":8080")
	site6vBase := env("SITE6V_BASE", "https://www.6v520.com")

	reg := plugin.NewRegistry()
	reg.Register(plugin.NewSite6V(site6vBase, 8))
	reg.Register(plugin.NewAPIBay())
	reg.Register(plugin.NewTorrentsCSV())
	// yts.mx TLS often fails behind current proxy; keep code for optional enable:
	if os.Getenv("ENABLE_YTS") == "1" {
		reg.Register(plugin.NewYTS())
	}

	eng := search.NewEngine(reg, 45*time.Second)
	srv := server.New(eng, reg, "web")

	log.Printf("magnet-agg listening on %s plugins=%v", addr, reg.Names())
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
