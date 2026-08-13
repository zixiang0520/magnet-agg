package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// APIBay is ThePirateBay apibay.org JSON API (public, no key).
type APIBay struct {
	base   string
	client *http.Client
}

func NewAPIBay() *APIBay {
	return NewAPIBayWith("", firstNonEmpty(os.Getenv("APIBAY_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("HTTP_PROXY")))
}

func NewAPIBayWith(base, proxy string) *APIBay {
	if base == "" {
		base = "https://apibay.org"
	}
	return &APIBay{base: strings.TrimRight(base, "/"), client: httpClient(proxy, 20*time.Second, false)}
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
	q = strings.TrimSpace(q)
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

	// apibay often returns a popular dump for CJK / empty matches — filter hard.
	cjk := hasCJK(q)
	tokens := asciiTokens(q)

	out := make([]Result, 0, len(items))
	for _, it := range items {
		if it.InfoHash == "" || it.Name == "" {
			continue
		}
		if it.ID.String() == "0" && it.InfoHash == "0000000000000000000000000000000000000000" {
			continue
		}
		if cjk {
			// For CJK queries, require at least one CJK rune from query in name,
			// OR skip entirely if name has zero CJK (English dump).
			if !nameMatchesCJKQuery(it.Name, q) {
				continue
			}
		} else if len(tokens) > 0 && !nameMatchesASCII(it.Name, tokens) {
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

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.In(r, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) {
			return true
		}
	}
	return false
}

func nameMatchesCJKQuery(name, q string) bool {
	// If name contains the full query string, ok.
	if strings.Contains(name, q) {
		return true
	}
	// Count overlapping Han runes (weak but blocks pure English dump).
	qRunes := make(map[rune]struct{})
	for _, r := range q {
		if unicode.In(r, unicode.Han) {
			qRunes[r] = struct{}{}
		}
	}
	if len(qRunes) == 0 {
		return false
	}
	hit := 0
	for _, r := range name {
		if _, ok := qRunes[r]; ok {
			hit++
		}
	}
	// require at least half of distinct query Han chars (min 1)
	need := len(qRunes)
	if need > 2 {
		need = need / 2
		if need < 2 {
			need = 2
		}
	}
	return hit >= need
}

func asciiTokens(q string) []string {
	fields := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) >= 2 {
			out = append(out, f)
		}
	}
	return out
}

func nameMatchesASCII(name string, tokens []string) bool {
	low := strings.ToLower(name)
	hit := 0
	for _, t := range tokens {
		if strings.Contains(low, t) {
			hit++
		}
	}
	// require majority of tokens
	need := (len(tokens) + 1) / 2
	if need < 1 {
		need = 1
	}
	return hit >= need
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
