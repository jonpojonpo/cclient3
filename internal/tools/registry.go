package tools

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jonpo/cclient3/internal/api"
)

// Registry holds all registered tools.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
	r.order = append(r.order, t.Name())
}

func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return t, nil
}

// APIDefs returns tool definitions for the API request.
func (r *Registry) APIDefs() []api.ToolDef {
	r.mu.RLock()
	defer r.mu.RUnlock()
	defs := make([]api.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		var schema interface{}
		json.Unmarshal(t.InputSchema(), &schema)
		defs = append(defs, api.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: schema,
		})
	}
	return defs
}
