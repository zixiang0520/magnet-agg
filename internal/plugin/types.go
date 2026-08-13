package plugin

import "context"

// Result is one magnet hit from any source.
type Result struct {
	Title    string `json:"title"`
	Magnet   string `json:"magnet"`
	InfoHash string `json:"info_hash,omitempty"`
	Size     string `json:"size,omitempty"`
	Source   string `json:"source"`
	PageURL  string `json:"page_url,omitempty"`
	Category string `json:"category,omitempty"`
	Seeders  int    `json:"seeders,omitempty"`
}

// Plugin is a searchable magnet source.
type Plugin interface {
	Name() string
	Search(ctx context.Context, q string) ([]Result, error)
}

// Registry holds enabled plugins.
type Registry struct {
	list []Plugin
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Register(p Plugin) { r.list = append(r.list, p) }

func (r *Registry) All() []Plugin { return append([]Plugin(nil), r.list...) }

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.list))
	for _, p := range r.list {
		out = append(out, p.Name())
	}
	return out
}
