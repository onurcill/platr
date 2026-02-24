package history

import (
	"encoding/json"
	"sync"
	"time"
)

// Entry represents a single gRPC call record.
type Entry struct {
	ID           string          `json:"id"`
	ConnectionID string          `json:"connectionId"`
	Service      string          `json:"service"`
	Method       string          `json:"method"`
	RequestBody  json.RawMessage `json:"requestBody"`
	ResponseBody json.RawMessage `json:"responseBody,omitempty"`
	Status       string          `json:"status"`
	DurationMs   int64           `json:"durationMs"`
	Timestamp    time.Time       `json:"timestamp"`
}

// Store is an in-memory, thread-safe history store.
type Store struct {
	mu      sync.RWMutex
	entries []*Entry
	index   map[string]*Entry
	maxSize int
}

// NewStore creates a new history store with a default max capacity.
func NewStore() *Store {
	return &Store{
		index:   make(map[string]*Entry),
		maxSize: 500,
	}
}

// Add saves a new entry and returns it with an assigned ID.
func (s *Store) Add(e Entry) *Entry {
	e.ID = generateID()
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.entries = append(s.entries, &e)
	s.index[e.ID] = &e

	// Trim oldest entries when over capacity.
	if len(s.entries) > s.maxSize {
		oldest := s.entries[0]
		s.entries = s.entries[1:]
		delete(s.index, oldest.ID)
	}

	return &e
}

// List returns all entries in reverse chronological order (newest first).
func (s *Store) List() []*Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Entry, len(s.entries))
	for i, e := range s.entries {
		out[len(s.entries)-1-i] = e
	}
	return out
}

// Get returns a single entry by ID.
func (s *Store) Get(id string) (*Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.index[id]
	return e, ok
}

// Delete removes a single entry by ID.
func (s *Store) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.index[id]; !ok {
		return false
	}
	delete(s.index, id)

	newEntries := s.entries[:0]
	for _, e := range s.entries {
		if e.ID != id {
			newEntries = append(newEntries, e)
		}
	}
	s.entries = newEntries
	return true
}

// Clear removes all entries.
func (s *Store) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	s.index = make(map[string]*Entry)
}
