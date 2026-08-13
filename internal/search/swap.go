package search

import (
	"time"

	"magnet-agg/internal/plugin"
)

func (e *Engine) SetRegistry(reg *plugin.Registry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.reg = reg
}

func (e *Engine) Registry() *plugin.Registry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.reg
}

func (e *Engine) Timeout() time.Duration { return e.timeout }
