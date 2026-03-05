package api

import "context"

// Provider is the interface all AI backend clients must implement.
type Provider interface {
	StreamMessage(ctx context.Context, req *Request, cb StreamCallback) error
	SendMessage(ctx context.Context, req *Request) (*Response, error)
	ListModels(ctx context.Context) ([]ModelInfo, error)
	Name() string
}

// ProviderRegistry holds named providers and tracks the default.
type ProviderRegistry struct {
	providers   map[string]Provider
	defaultName string
}

// NewProviderRegistry creates a registry with the given provider as default.
func NewProviderRegistry(def Provider) *ProviderRegistry {
	r := &ProviderRegistry{
		providers:   map[string]Provider{},
		defaultName: def.Name(),
	}
	r.providers[def.Name()] = def
	return r
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(p Provider) {
	r.providers[p.Name()] = p
}

// Get returns a provider by name, falling back to the default if not found.
func (r *ProviderRegistry) Get(name string) Provider {
	if name == "" {
		return r.providers[r.defaultName]
	}
	if p, ok := r.providers[name]; ok {
		return p
	}
	return r.providers[r.defaultName]
}

// Has returns true if a provider name is registered.
func (r *ProviderRegistry) Has(name string) bool {
	_, ok := r.providers[name]
	return ok
}

// Default returns the default provider.
func (r *ProviderRegistry) Default() Provider {
	return r.providers[r.defaultName]
}

// DefaultName returns the name of the default provider.
func (r *ProviderRegistry) DefaultName() string {
	return r.defaultName
}

// SetDefault changes the default provider. Returns false if the name is unknown.
func (r *ProviderRegistry) SetDefault(name string) bool {
	if _, ok := r.providers[name]; ok {
		r.defaultName = name
		return true
	}
	return false
}

// Names returns all registered provider names.
func (r *ProviderRegistry) Names() []string {
	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}
