package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Client struct {
	apiKey string
	lang   string
	base   string
	http   *http.Client
}

func New(apiKey, proxy, lang string) *Client {
	if lang == "" {
		lang = "zh-CN"
	}
	transport := &http.Transport{}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}
	return &Client{
		apiKey: apiKey,
		lang:   lang,
		base:   "https://api.themoviedb.org",
		http:   &http.Client{Transport: transport, Timeout: 15 * time.Second},
	}
}

type Result struct {
	Title     string
	Date      string
	MediaType string // movie / tv
}

type Item struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	Date        string  `json:"date"`
	Year        string  `json:"year"`
	MediaType   string  `json:"media_type"`
	Overview    string  `json:"overview,omitempty"`
	Vote        float64 `json:"vote,omitempty"`
	Poster      string  `json:"poster,omitempty"`
	SearchQuery string  `json:"search_query"`
}

func (c *Client) Search(ctx context.Context, query, mediaType string) (*Result, error) {
	if c == nil || c.apiKey == "" {
		return nil, errors.New("tmdb api key 未配置")
	}
	if mediaType == "" {
		mediaType = "movie"
	}
	u := fmt.Sprintf("%s/3/search/%s?api_key=%s&query=%s&language=%s",
		c.base, mediaType, c.apiKey, url.QueryEscape(query), c.lang)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tmdb status %d", resp.StatusCode)
	}
	var data struct {
		Results []struct {
			Title        string `json:"title"`
			Name         string `json:"name"`
			ReleaseDate  string `json:"release_date"`
			FirstAirDate string `json:"first_air_date"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	if len(data.Results) == 0 {
		return nil, nil
	}
	r := data.Results[0]
	title := r.Title
	if title == "" {
		title = r.Name
	}
	date := r.ReleaseDate
	if date == "" {
		date = r.FirstAirDate
	}
	return &Result{Title: title, Date: date, MediaType: mediaType}, nil
}

func (c *Client) SearchAuto(ctx context.Context, query string, preferTV bool) (*Result, error) {
	order := []string{"movie", "tv"}
	if preferTV {
		order = []string{"tv", "movie"}
	}
	var firstErr error
	for _, mt := range order {
		r, err := c.Search(ctx, query, mt)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if r != nil && r.Title != "" {
			return r, nil
		}
	}
	return nil, firstErr
}

var yearRe = regexp.MustCompile(`^(\d{4})`)

func YearFromDate(s string) string {
	if m := yearRe.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func (c *Client) Discover(ctx context.Context, mediaType string, limit int) ([]Item, error) {
	if c == nil || c.apiKey == "" {
		return nil, errors.New("tmdb api key 未配置")
	}
	if mediaType != "tv" {
		mediaType = "movie"
	}
	if limit <= 0 || limit > 20 {
		limit = 12
	}
	u := fmt.Sprintf("%s/3/movie/now_playing?api_key=%s&language=%s&page=1&region=CN",
		c.base, c.apiKey, c.lang)
	if mediaType == "tv" {
		u = fmt.Sprintf("%s/3/trending/tv/week?api_key=%s&language=%s",
			c.base, c.apiKey, c.lang)
	}
	items, err := c.decodeList(ctx, u, mediaType)
	if err != nil || len(items) >= limit {
		if len(items) > limit {
			items = items[:limit]
		}
		return items, err
	}
	// fallback: popular if now_playing / on_the_air is thin
	u2 := fmt.Sprintf("%s/3/%s/popular?api_key=%s&language=%s&page=1",
		c.base, mediaType, c.apiKey, c.lang)
	more, err2 := c.decodeList(ctx, u2, mediaType)
	if err2 != nil && len(items) == 0 {
		return nil, err2
	}
	seen := map[int]bool{}
	for _, it := range items {
		seen[it.ID] = true
	}
	for _, it := range more {
		if seen[it.ID] {
			continue
		}
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (c *Client) decodeList(ctx context.Context, rawURL, mediaType string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tmdb status %d", resp.StatusCode)
	}
	var data struct {
		Results []struct {
			ID           int     `json:"id"`
			Title        string  `json:"title"`
			Name         string  `json:"name"`
			ReleaseDate  string  `json:"release_date"`
			FirstAirDate string  `json:"first_air_date"`
			Overview     string  `json:"overview"`
			VoteAverage  float64 `json:"vote_average"`
			PosterPath   string  `json:"poster_path"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(data.Results))
	for _, r := range data.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		if title == "" {
			continue
		}
		date := r.ReleaseDate
		if date == "" {
			date = r.FirstAirDate
		}
		year := YearFromDate(date)
		q := title
		if year != "" {
			q = title + " " + year
		}
		poster := ""
		if r.PosterPath != "" {
			poster = "/api/tmdb/poster?path=" + url.QueryEscape(r.PosterPath)
		}
		out = append(out, Item{
			ID: r.ID, Title: title, Date: date, Year: year, MediaType: mediaType,
			Overview: r.Overview, Vote: r.VoteAverage, Poster: poster, SearchQuery: q,
		})
	}
	return out, nil
}

func (c *Client) FetchPoster(ctx context.Context, posterPath string) (*http.Response, error) {
	if c == nil {
		return nil, errors.New("tmdb 未配置")
	}
	posterPath = strings.TrimSpace(posterPath)
	if posterPath == "" || strings.Contains(posterPath, "..") {
		return nil, errors.New("无效海报路径")
	}
	if !strings.HasPrefix(posterPath, "/") {
		posterPath = "/" + posterPath
	}
	u := "https://image.tmdb.org/t/p/w342" + posterPath
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	return c.http.Do(req)
}
