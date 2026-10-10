package registry

import (
	"path/filepath"
	"sync"
)

// EnvKey is the reserved config key under which the executor passes the
// run's Env to every factory. Composites forward it to their children.
const EnvKey = "_env"

// Env is per-run context for generator factories: the directory relative
// file paths resolve against, and a cache so each external value source
// (file or URL) is read once per run however many chunks build generators.
type Env struct {
	baseDir string
	mu      sync.Mutex
	sources map[string][]any
}

// NewEnv creates an Env whose relative paths resolve against baseDir
// (typically the plan file's directory; "" means the working directory).
func NewEnv(baseDir string) *Env {
	return &Env{baseDir: baseDir, sources: make(map[string][]any)}
}

// envFrom returns the Env passed in config, or nil outside a run.
func envFrom(config map[string]any) *Env {
	env, _ := config[EnvKey].(*Env)
	return env
}

// resolvePath anchors a relative path at the base directory.
func (e *Env) resolvePath(path string) string {
	if e == nil || e.baseDir == "" || filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(e.baseDir, path)
}

// loadOnce returns the cached values for key, calling fetch on first use.
// The lock is held during fetch so concurrent chunks never fetch twice.
func (e *Env) loadOnce(key string, fetch func() ([]any, error)) ([]any, error) {
	if e == nil {
		return fetch()
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if values, ok := e.sources[key]; ok {
		return values, nil
	}
	values, err := fetch()
	if err != nil {
		return nil, err
	}
	e.sources[key] = values
	return values, nil
}
