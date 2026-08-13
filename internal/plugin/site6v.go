package plugin

import (
	"context"
	"log"
	"sync"

	site6v "magnet-agg/plugins/site6v"
)

// Site6V wraps the existing 6v520 client as a magnet-agg plugin.
type Site6V struct {
	client    *site6v.Client
	maxDetail int
}

func NewSite6V(base string, maxDetail int) *Site6V {
	if maxDetail <= 0 {
		maxDetail = 8
	}
	if base == "" {
		base = "https://www.6v520.com"
	}
	return &Site6V{client: site6v.NewClient(base), maxDetail: maxDetail}
}

func (p *Site6V) Name() string { return "6v520" }

func (p *Site6V) Search(ctx context.Context, q string) ([]Result, error) {
	resources := p.client.Search(ctx, q, 3)
	if len(resources) == 0 {
		return nil, nil
	}
	if len(resources) > p.maxDetail {
		resources = resources[:p.maxDetail]
	}

	type pair struct {
		r  site6v.Resource
		ms []site6v.Magnet
	}
	ch := make(chan pair, len(resources))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, res := range resources {
		res := res
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-ctx.Done():
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()
			ms, err := p.client.FetchMagnets(ctx, res.URL)
			if err != nil {
				log.Printf("6v520 magnets %s: %v", res.URL, err)
				ch <- pair{r: res}
				return
			}
			ch <- pair{r: res, ms: ms}
		}()
	}
	go func() { wg.Wait(); close(ch) }()

	var out []Result
	for item := range ch {
		if len(item.ms) == 0 {
			continue
		}
		for _, m := range item.ms {
			magnet := m.Magnet
			title := m.Name
			if title == "" {
				title = item.r.Title
			} else {
				title = item.r.Title + " | " + title
			}
			out = append(out, Result{
				Title:    title,
				Magnet:   magnet,
				InfoHash: InfoHashFromMagnet(magnet),
				Source:   p.Name(),
				PageURL:  item.r.URL,
				Category: item.r.Category,
			})
		}
	}
	return out, nil
}
