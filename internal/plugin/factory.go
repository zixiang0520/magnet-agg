package plugin

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// KnownPlugins is the catalog shown in admin (even when disabled).
var KnownPlugins = []string{"6v520", "apibay", "torrents-csv", "yts"}

type PluginSpec struct {
	Name    string
	Enabled bool
	Base    string
	Proxy   string
}

func Build(specs []PluginSpec) *Registry {
	reg := NewRegistry()
	for _, s := range specs {
		if !s.Enabled {
			continue
		}
		if p := newFromSpec(s); p != nil {
			reg.Register(p)
		}
	}
	return reg
}

func newFromSpec(s PluginSpec) Plugin {
	switch s.Name {
	case "6v520":
		return NewSite6V(s.Base, 8)
	case "apibay":
		return NewAPIBayWith(s.Base, s.Proxy)
	case "torrents-csv":
		return NewTorrentsCSVWith(s.Base, s.Proxy)
	case "yts":
		return NewYTSWith(s.Base, s.Proxy)
	default:
		return nil
	}
}

func httpClient(proxy string, timeout time.Duration, forceDirect bool) *http.Client {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if forceDirect {
		tr.Proxy = nil
		tr.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
		tr.TLSHandshakeTimeout = 15 * time.Second
	} else if strings.TrimSpace(proxy) != "" {
		if u, err := url.Parse(strings.TrimSpace(proxy)); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	} else {
		tr.Proxy = http.ProxyFromEnvironment
	}
	return &http.Client{Timeout: timeout, Transport: tr}
}

// Probe runs a short search to test connectivity.
func Probe(ctx context.Context, spec PluginSpec, q string) (int, error) {
	p := newFromSpec(spec)
	if p == nil {
		return 0, nil
	}
	if q == "" {
		q = "inception"
		if spec.Name == "6v520" {
			q = "三体"
		}
	}
	rs, err := p.Search(ctx, q)
	if err != nil {
		return 0, err
	}
	return len(rs), nil
}
