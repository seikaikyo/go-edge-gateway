package plugin

import "fmt"

// Factory is a constructor that returns a new Plugin instance.
type Factory func() Plugin

var registry = map[string]Factory{}

// Register adds a plugin factory to the global registry.
// Typically called from each plugin's init() function.
func Register(name string, f Factory) {
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("plugin %q already registered", name))
	}
	registry[name] = f
}

// Get returns the factory for a named plugin, or nil if not registered.
func Get(name string) Factory {
	return registry[name]
}

// All returns every registered plugin name.
func All() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	return names
}
