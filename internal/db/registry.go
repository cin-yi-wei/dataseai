package db

import (
	"fmt"
	"sync"
)

var (
	regMu      sync.RWMutex
	registered = map[Engine]Dialect{}
)

// Register installs a dialect. Engines call this from their package init.
func Register(e Engine, d Dialect) {
	regMu.Lock()
	defer regMu.Unlock()
	registered[e] = d
}

// Lookup returns the dialect for an engine, ok=false if missing.
func Lookup(e Engine) (Dialect, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	d, ok := registered[e]
	return d, ok
}

// MustGet returns the dialect for an engine and panics if absent. Use only
// in startup paths where a missing dialect is a programmer error.
func MustGet(e Engine) Dialect {
	d, ok := Lookup(e)
	if !ok {
		panic(fmt.Sprintf("db: no dialect registered for engine %q", e))
	}
	return d
}
