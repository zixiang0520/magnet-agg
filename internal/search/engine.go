package search

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"magnet-agg/internal/plugin"
)

type Engine struct {
	mu      sync.RWMutex
	reg     *plugin.Registry
	timeout time.Duration
}

func NewEngine(reg *plugin.Registry, timeout time.Duration) *Engine {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &Engine{reg: reg, timeout: timeout}
}

type Response struct {
	Query   string            `json:"query"`
	Total   int               `json:"total"`
	TookMs  int64             `json:"took_ms"`
	Results []plugin.Result   `json:"results"`
	Sources map[string]int    `json:"sources"`
	Errors  map[string]string `json:"errors,omitempty"`
}

func (e *Engine) Search(ctx context.Context, q string) *Response {
	q = strings.TrimSpace(q)
	start := time.Now()
	out := &Response{
		Query:   q,
		Sources: map[string]int{},
		Errors:  map[string]string{},
	}
	if q == "" {
		return out
	}
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	plugins := e.Registry().All()
	var mu sync.Mutex
	var all []plugin.Result
	var wg sync.WaitGroup
	for _, p := range plugins {
		p := p
		wg.Add(1)
		go func() {
			defer wg.Done()
			rs, err := p.Search(ctx, q)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				log.Printf("plugin %s error: %v", p.Name(), err)
				out.Errors[p.Name()] = err.Error()
				return
			}
			out.Sources[p.Name()] = len(rs)
			all = append(all, rs...)
		}()
	}
	wg.Wait()

	merged := dedupe(all)
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Seeders != merged[j].Seeders {
			return merged[i].Seeders > merged[j].Seeders
		}
		if merged[i].Source != merged[j].Source {
			if merged[i].Source == "6v520" {
				return true
			}
			if merged[j].Source == "6v520" {
				return false
			}
		}
		return merged[i].Title < merged[j].Title
	})
	out.Results = merged
	out.Total = len(merged)
	out.TookMs = time.Since(start).Milliseconds()
	if len(out.Errors) == 0 {
		out.Errors = nil
	}
	return out
}

func dedupe(in []plugin.Result) []plugin.Result {
	seen := map[string]int{}
	out := make([]plugin.Result, 0, len(in))
	for _, r := range in {
		key := r.InfoHash
		if key == "" {
			key = plugin.InfoHashFromMagnet(r.Magnet)
			r.InfoHash = key
		}
		if key == "" {
			key = "m:" + strings.ToLower(r.Magnet)
		}
		if key == "m:" {
			key = "t:" + strings.ToLower(r.Title) + "|" + r.Source
		}
		if idx, ok := seen[key]; ok {
			cur := out[idx]
			if r.Seeders > cur.Seeders || (cur.Size == "" && r.Size != "") {
				if cur.Source != r.Source && !strings.Contains(cur.Source, r.Source) {
					r.Source = cur.Source + "+" + r.Source
				}
				out[idx] = r
			} else if cur.Source != r.Source && !strings.Contains(cur.Source, r.Source) {
				cur.Source = cur.Source + "+" + r.Source
				out[idx] = cur
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, r)
	}
	return out
}
