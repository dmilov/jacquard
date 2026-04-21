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

func (r *Registry) SetNeedsInput(id string, v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.looms[id]; ok {
		l.NeedsInput = v
		r.looms[id] = l
	}
}

func (r *Registry) Rename(id, name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	l, ok := r.looms[id]
	if !ok {
		return false
	}
	l.Name = name
	r.looms[id] = l
	return true
}

func (r *Registry) FindByConversationID(convID string) (models.LoomInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, l := range r.looms {
		if l.ConversationID == convID {
			return l, true
		}
	}
	return models.LoomInfo{}, false
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
