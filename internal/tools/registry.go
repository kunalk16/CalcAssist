package tools

// Registry holds the set of available tools in a stable, insertion-ordered list.
type Registry struct {
	order  []string
	byName map[string]Tool
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{byName: make(map[string]Tool)}
}

// Register adds (or replaces) a tool. Insertion order is preserved for listing.
func (r *Registry) Register(t Tool) {
	if _, exists := r.byName[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.byName[t.Name()] = t
}

// Get returns the tool with the given name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.byName[name]
	return t, ok
}

// All returns the registered tools in insertion order.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.byName[name])
	}
	return out
}

// Len reports the number of registered tools.
func (r *Registry) Len() int { return len(r.order) }
