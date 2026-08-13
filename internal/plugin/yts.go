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

// YTS is the YTS.mx public movie API (JSON, magnets per quality).
// Third aggregate source — strong for English movie titles / IMDB-style queries.
type YTS struct {
	base   string
	client *http.Client
}

func NewYTS() *YTS {
	return NewYTSWith(os.Getenv("YTS_BASE"), firstNonEmpty(os.Getenv("YTS_PROXY"), os.Getenv("APIBAY_PROXY"), os.Getenv("HTTPS_PROXY"), os.Getenv("HTTP_PROXY")))
}

func NewYTSWith(base, proxy string) *YTS {
	if base == "" {
		base = firstNonEmpty(os.Getenv("YTS_BASE"), "https://yts.mx/api/v2")
	}
	return &YTS{base: strings.TrimRight(base, "/"), client: httpClient(proxy, 20*time.Second, false)}
}

func (p *YTS) Name() string { return "yts" }

type ytsResponse struct {
	Status string `json:"status"`
	Data   struct {
		MovieCount int `json:"movie_count"`
		Movies     []struct {
			TitleLong string `json:"title_long"`
			Title     string `json:"title"`
			Year      int    `json:"year"`
			URL       string `json:"url"`
			Torrents  []struct {
				Hash     string `json:"hash"`
				Quality  string `json:"quality"`
				Type     string `json:"type"`
				Size     string `json:"size"`
				Seeds    int    `json:"seeds"`
				Peers    int    `json:"peers"`
				VideoCodec string `json:"video_codec"`
			} `json:"torrents"`
		} `json:"movies"`
	} `json:"data"`
}

func (p *YTS) Search(ctx context.Context, q string) ([]Result, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	// Skip pure CJK-only queries — YTS catalogue is English-centric; avoid noise.
	if hasCJK(q) && len(asciiTokens(q)) == 0 {
		return nil, nil
	}

	u := fmt.Sprintf("%s/list_movies.json?query_term=%s&limit=20&sort_by=seeds",
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
		return nil, fmt.Errorf("yts status %d", resp.StatusCode)
	}
	var body ytsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if !strings.EqualFold(body.Status, "ok") {
		return nil, fmt.Errorf("yts status %q", body.Status)
	}

	var out []Result
	for _, m := range body.Data.Movies {
		title := strings.TrimSpace(m.TitleLong)
		if title == "" {
			title = strings.TrimSpace(m.Title)
			if m.Year > 0 {
				title = fmt.Sprintf("%s (%d)", title, m.Year)
			}
		}
		for _, t := range m.Torrents {
			if t.Hash == "" {
				continue
			}
			dn := title
			if t.Quality != "" {
				dn = title + " " + t.Quality
				if t.Type != "" {
					dn += " " + t.Type
				}
			}
			magnet := MagnetFromHash(t.Hash, dn)
			label := title
			if t.Quality != "" {
				label = fmt.Sprintf("%s | %s", title, t.Quality)
				if t.Type != "" {
					label += " " + t.Type
				}
				if t.VideoCodec != "" {
					label += " " + t.VideoCodec
				}
			}
			out = append(out, Result{
				Title:    label,
				Magnet:   magnet,
				InfoHash: InfoHashFromMagnet(magnet),
				Size:     t.Size,
				Source:   p.Name(),
				Seeders:  t.Seeds,
				PageURL:  m.URL,
				Category: "movie",
			})
			if len(out) >= 40 {
				return out, nil
			}
		}
	}
	return out, nil
}
