package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
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
