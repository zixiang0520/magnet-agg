package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// APIBay is ThePirateBay apibay.org JSON API (public, no key).
type APIBay struct {
	base   string
	client *http.Client
}

func NewAPIBay() *APIBay {
	return &APIBay{
		base: "https://apibay.org",
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (p *APIBay) Name() string { return "apibay" }

type apibayItem struct {
	ID       json.Number `json:"id"`
	Name     string      `json:"name"`
	InfoHash string      `json:"info_hash"`
	Leechers json.Number `json:"leechers"`
	Seeders  json.Number `json:"seeders"`
	Size     json.Number `json:"size"`
	Category json.Number `json:"category"`
}

func (p *APIBay) Search(ctx context.Context, q string) ([]Result, error) {
	u := fmt.Sprintf("%s/q.php?q=%s", p.base, url.QueryEscape(q))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "magnet-agg/0.1")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("apibay status %d", resp.StatusCode)
	}
	var items []apibayItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(items))
	for _, it := range items {
		if it.InfoHash == "" || it.Name == "" {
			continue
		}
		if it.ID.String() == "0" && it.InfoHash == "0000000000000000000000000000000000000000" {
			continue
		}
		sizeN, _ := it.Size.Int64()
		seed, _ := it.Seeders.Int64()
		magnet := MagnetFromHash(it.InfoHash, it.Name)
		out = append(out, Result{
			Title:    it.Name,
			Magnet:   magnet,
			InfoHash: InfoHashFromMagnet(magnet),
			Size:     humanSize(sizeN),
			Source:   p.Name(),
			Seeders:  int(seed),
			PageURL:  fmt.Sprintf("https://apibay.org/t/%s", it.ID.String()),
		})
		if len(out) >= 40 {
			break
		}
	}
	return out, nil
}

func humanSize(n int64) string {
	if n <= 0 {
		return ""
	}
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + " B"
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
