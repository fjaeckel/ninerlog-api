package provider

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds the set of providers known to this binary. It is intended to
// be constructed once at startup and treated as read-only thereafter.
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]Provider)}
}

// Register adds a provider. Duplicate names panic; this is a startup-time
// programming error.
func (r *Registry) Register(p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil {
		panic("cloudbackup/provider: cannot register nil provider")
	}
	name := p.Name()
	if name == "" {
		panic("cloudbackup/provider: provider with empty Name()")
	}
	if _, exists := r.providers[name]; exists {
		panic(fmt.Sprintf("cloudbackup/provider: duplicate provider %q", name))
	}
	r.providers[name] = p
}

// Get returns the provider by name, or (nil, false) if absent.
func (r *Registry) Get(name string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[name]
	return p, ok
}

// MustGet panics if the provider is not registered. Use only in code paths
// where the name has already been validated against the registry.
func (r *Registry) MustGet(name string) Provider {
	p, ok := r.Get(name)
	if !ok {
		panic(fmt.Sprintf("cloudbackup/provider: provider %q not registered", name))
	}
	return p
}

// Names returns the registered provider names sorted alphabetically. The slice
// is a fresh copy and safe to mutate.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// All returns the registered providers in stable (name-sorted) order.
func (r *Registry) All() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.providers))
	for k := range r.providers {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]Provider, 0, len(names))
	for _, n := range names {
		out = append(out, r.providers[n])
	}
	return out
}
