package runtime

import (
	"sort"
	"sync"
)

// The registry maps names to adapters. Adapters self-register: no
// production code in this package registers anything, so support for a
// runtime exists only with an implementation behind it. Today that means
// no Go adapter at all — the product-supported OpenCode runtime lives in
// the existing TypeScript plugin (.opencode/plugins/regulus.ts).
var (
	registryMu       sync.Mutex
	registryAdapters = make(map[string]Adapter)
)

// Register makes an adapter discoverable under name, replacing any
// previous entry. It panics on an empty name or a nil adapter: both are
// programming errors, and silent acceptance would fake support.
func Register(name string, a Adapter) {
	if name == "" {
		panic("runtime: Register with empty adapter name")
	}
	if a == nil {
		panic("runtime: Register nil Adapter")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registryAdapters[name] = a
}

// Lookup returns the adapter registered under name. An unknown name is
// refused (ok == false), never answered with a stand-in.
func Lookup(name string) (Adapter, bool) {
	registryMu.Lock()
	defer registryMu.Unlock()
	a, ok := registryAdapters[name]
	return a, ok
}

// Names returns the registered adapter names in sorted order.
func Names() []string {
	registryMu.Lock()
	defer registryMu.Unlock()
	names := make([]string, 0, len(registryAdapters))
	for name := range registryAdapters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
