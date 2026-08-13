package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// TorrentsCSV is https://torrents-csv.com public search API (JSON, no key).
// Third aggregate source — works via proxy where yts.mx TLS often fails on this network.
type TorrentsCSV struct {
	base   string
	client *http.Client
}

func NewTorrentsCSV() *TorrentsCSV {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	proxyURL := firstNonEmpty(
		os.Getenv("TORRENTSCSV_PROXY"),
		os.Getenv("APIBAY_PROXY"),
		os.Getenv("HTTPS_PROXY"),
		os.Getenv("HTTP_PROXY"),
	)
	if proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	} else {
		transport.Proxy = http.ProxyFromEnvironment
	}
	return &TorrentsCSV{
		base: firstNonEmpty(os.Getenv("TORRENTSCSV_BASE"), "https://torrents-csv.com"),
		client: &http.Client{
			Timeout:   20 * time.Second,
			Transport: transport,
		},
	}
}

func (p *TorrentsCSV) Name() string { return "torrents-csv" }

type torrentsCSVResp struct {
	Torrents []struct {
		InfoHash string `json:"infohash"`
		Name     string `json:"name"`
		Size     int64  `json:"size_bytes"`
		Seeders  int    `json:"seeders"`
		Leechers int    `json:"leechers"`
	} `json:"torrents"`
}

func (p *TorrentsCSV) Search(ctx context.Context, q string) ([]Result, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	u := fmt.Sprintf("%s/service/search?q=%s&size=40",
		strings.TrimRight(p.base, "/"), url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "magnet-agg/0.1")
	req.Header.Set("Accept", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("torrents-csv status %d", resp.StatusCode)
	}
	var body torrentsCSVResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	cjk := hasCJK(q)
	tokens := asciiTokens(q)
	out := make([]Result, 0, len(body.Torrents))
	for _, t := range body.Torrents {
		if t.InfoHash == "" || t.Name == "" {
			continue
		}
		if cjk {
			if !nameMatchesCJKQuery(t.Name, q) {
				continue
			}
		} else if len(tokens) > 0 && !nameMatchesASCII(t.Name, tokens) {
			continue
		}
		magnet := MagnetFromHash(t.InfoHash, t.Name)
		out = append(out, Result{
			Title:    t.Name,
			Magnet:   magnet,
			InfoHash: InfoHashFromMagnet(magnet),
			Size:     humanSize(t.Size),
			Source:   p.Name(),
			Seeders:  t.Seeders,
			PageURL:  fmt.Sprintf("%s/#/search/%s", strings.TrimRight(p.base, "/"), url.PathEscape(q)),
		})
		if len(out) >= 40 {
			break
		}
	}
	return out, nil
}
