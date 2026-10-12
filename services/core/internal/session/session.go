// Package session tracks active lease state, with optional file-backed persistence.
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Lease records the state of a single provisioned session.
type Lease struct {
	ID               string    `json:"id"`
	Template         string    `json:"template"`
	ExpiresAt        time.Time `json:"expires_at"`
	CeilingUSD       float64   `json:"ceiling_usd"`
	SettledSoFarUSD  float64   `json:"settled_so_far_usd"`
	IdempotencyKey   string    `json:"idempotency_key,omitempty"`
}

// Store is an in-memory lease registry with optional JSONL persistence.
// When DatabasePath is non-empty it writes new leases to that file and
// reloads them on startup, so sessions survive process restarts.
type Store struct {
	mu               sync.Mutex
	leases           map[string]*Lease
	idempotencyIndex map[string]*Lease // idempotency key → lease
	DatabasePath     string
	f                *os.File
}

// NewStore returns a purely in-memory store.
func NewStore() *Store {
	s := &Store{
		leases:           make(map[string]*Lease),
		idempotencyIndex: make(map[string]*Lease),
	}
	go s.reapLoop()
	return s
}

// NewPersistentStore returns a Store backed by a JSONL file.
// Existing, non-expired leases are loaded on startup.
func NewPersistentStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("session store: create dir: %w", err)
	}
	s := &Store{
		leases:           make(map[string]*Lease),
		idempotencyIndex: make(map[string]*Lease),
		DatabasePath:     path,
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("session store: open for append: %w", err)
	}
	s.f = f
	go s.reapLoop()
	return s, nil
}

// load reads the JSONL file and interns non-expired leases.
func (s *Store) load() error {
	f, err := os.Open(s.DatabasePath)
	if os.IsNotExist(err) {
		return nil // first start; fine.
	}
	if err != nil {
		return fmt.Errorf("session store: open for load: %w", err)
	}
	defer f.Close()

	now := time.Now()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var l Lease
		if err := json.Unmarshal(scanner.Bytes(), &l); err != nil {
			continue // skip corrupt lines
		}
		if l.ExpiresAt.After(now) {
			s.leases[l.ID] = &l
			if l.IdempotencyKey != "" {
				s.idempotencyIndex[l.IdempotencyKey] = &l
			}
		}
	}
	return scanner.Err()
}

// append writes a lease to the backing file, if one is open.
func (s *Store) append(l *Lease) {
	if s.f == nil {
		return
	}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = s.f.Write(b)
}

func (s *Store) Put(l *Lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[l.ID] = l
	if l.IdempotencyKey != "" {
		s.idempotencyIndex[l.IdempotencyKey] = l
	}
	s.append(l)
}

func (s *Store) Get(id string) (*Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.leases[id]
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	if time.Now().After(l.ExpiresAt) {
		return nil, fmt.Errorf("session %s expired", id)
	}
	return l, nil
}

// GetByIdempotencyKey returns the lease for the given idempotency key, or nil
// if no matching non-expired lease exists. Callers use this to return the
// original response on retried provision_env calls.
func (s *Store) GetByIdempotencyKey(key string) *Lease {
	if key == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.idempotencyIndex[key]
	if !ok {
		return nil
	}
	if time.Now().After(l.ExpiresAt) {
		return nil // expired; treat as absent
	}
	return l
}

func (s *Store) Extend(id string, additionalUSD float64, newExpiry time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.leases[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	l.CeilingUSD += additionalUSD
	l.ExpiresAt = newExpiry
	s.append(l) // persist the updated state
	return nil
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.leases[id]; ok && l.IdempotencyKey != "" {
		delete(s.idempotencyIndex, l.IdempotencyKey)
	}
	delete(s.leases, id)
}

func (s *Store) reapLoop() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, l := range s.leases {
			if now.After(l.ExpiresAt) {
				if l.IdempotencyKey != "" {
					delete(s.idempotencyIndex, l.IdempotencyKey)
				}
				delete(s.leases, id)
			}
		}
		s.mu.Unlock()
	}
}
