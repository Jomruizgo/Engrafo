// Package watcher observes git post-commit events for incremental graph updates.
// Full implementation: feature/graph-builder.
package watcher

// Watcher triggers graph updates on git post-commit.
type Watcher struct{}

// New creates a Watcher for the given repository root.
func New(_ string) *Watcher {
	return &Watcher{}
}

// Run blocks until the process exits, triggering graph updates on each commit.
func (w *Watcher) Run() error {
	return nil
}
