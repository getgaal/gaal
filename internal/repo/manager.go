package repo

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"gaal/internal/config"
	"gaal/internal/core/vcs"
)

// Status holds the sync state of a single repository.
type Status struct {
	Path    string
	Type    string
	URL     string
	Version string // configured version
	Current string // current local version
	Cloned  bool
	Dirty   bool // true when local modifications are detected
	Err     error
}

// Manager handles the synchronisation of all repositories.
type Manager struct {
	repos    map[string]config.ConfigRepo
	stateDir string // reserved for future snapshot use
}

// NewManager creates a new repository Manager.
func NewManager(repos map[string]config.ConfigRepo, stateDir string) *Manager {
	return &Manager{repos: repos, stateDir: stateDir}
}

// Sync clones or updates every repository in parallel.
func (m *Manager) Sync(ctx context.Context) error {
	if len(m.repos) == 0 {
		return nil
	}

	errCh := make(chan error, len(m.repos))
	var wg sync.WaitGroup

	for path, cfg := range m.repos {
		wg.Add(1)
		go func(path string, cfg config.ConfigRepo) {
			defer wg.Done()
			if err := m.syncOne(ctx, path, cfg); err != nil {
				errCh <- fmt.Errorf("repo %q: %w", path, err)
			}
		}(path, cfg)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return fmt.Errorf("%d repository error(s): %v", len(errs), errs)
	}
	return nil
}

func (m *Manager) syncOne(ctx context.Context, path string, cfg config.ConfigRepo) error {
	slog.DebugContext(ctx, "syncing repository", "path", path, "type", cfg.Type, "version", cfg.Version)
	backend, err := vcs.New(cfg.Type)
	if err != nil {
		return err
	}

	if !backend.IsCloned(path) {
		slog.Info("cloning repository", "path", path, "url", cfg.URL, "version", cfg.Version)
		return backend.Clone(ctx, cfg.URL, path, cfg.Version)
	}

	slog.Info("updating repository", "path", path)
	return backend.Update(ctx, path, cfg.Version)
}

// Status returns the current status of every repository.
func (m *Manager) Status(ctx context.Context) []Status {
	statuses := make([]Status, 0, len(m.repos))

	var mu sync.Mutex
	var wg sync.WaitGroup

	for path, cfg := range m.repos {
		wg.Add(1)
		go func(path string, cfg config.ConfigRepo) {
			defer wg.Done()

			st := Status{
				Path:    path,
				Type:    cfg.Type,
				URL:     cfg.URL,
				Version: cfg.Version,
			}

			backend, err := vcs.New(cfg.Type)
			if err != nil {
				st.Err = err
				mu.Lock()
				statuses = append(statuses, st)
				mu.Unlock()
				return
			}

			st.Cloned = backend.IsCloned(path)
			if st.Cloned {
				st.Current, st.Err = backend.CurrentVersion(ctx, path)
				if st.Err == nil {
					st.Dirty, st.Err = backend.HasChanges(ctx, path)
				}
			}

			mu.Lock()
			statuses = append(statuses, st)
			mu.Unlock()
		}(path, cfg)
	}

	wg.Wait()
	return statuses
}
