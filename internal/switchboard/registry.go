package switchboard

import (
	"sync"

	"github.com/dmilov/jacquard/internal/models"
)

// Registry is a thread-safe in-memory store of active Loom instances.
type Registry struct {
	mu    sync.RWMutex
	looms map[string]models.LoomInfo
}

func NewRegistry() *Registry {
	return &Registry{looms: make(map[string]models.LoomInfo)}
}

func (r *Registry) Register(loom models.LoomInfo) {
	r.mu.Lock()
	r.looms[loom.ID] = loom
	r.mu.Unlock()
}

func (r *Registry) Deregister(id string) {
	r.mu.Lock()
	delete(r.looms, id)
	r.mu.Unlock()
}

func (r *Registry) Get(id string) (models.LoomInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.looms[id]
	return l, ok
}

func (r *Registry) List() []models.LoomInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]models.LoomInfo, 0, len(r.looms))
	for _, l := range r.looms {
		result = append(result, l)
	}
	return result
}
