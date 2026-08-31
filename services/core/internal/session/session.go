// Package session tracks lease state in memory. Fine for the hackathon
// MVP and local debugging; NOT durable across restarts. Swap the store
// for something persistent before this is a real business (SPEC-100
// doesn't mandate a specific store — this is a deliberate MVP shortcut,
// noted here so nobody mistakes it for a permanent decision).
package session

import (
	"fmt"
	"sync"
	"time"
)

type Lease struct {
	ID              string
	Template        string
	ExpiresAt       time.Time
	CeilingUSD      float64
	SettledSoFarUSD float64
}

type Store struct {
	mu      sync.Mutex
	leases  map[string]*Lease
}

func NewStore() *Store {
	s := &Store{leases: make(map[string]*Lease)}
	go s.reapLoop()
	return s
}

func (s *Store) Put(l *Lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[l.ID] = l
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

func (s *Store) Extend(id string, additionalUSD float64, newExpiry time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, ok := s.leases[id]
	if !ok {
		return fmt.Errorf("session %s not found", id)
	}
	l.CeilingUSD += additionalUSD
	l.ExpiresAt = newExpiry
	return nil
}

func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.leases, id)
}

// reapLoop clears expired leases periodically. This is a memory
// housekeeping sweep only — it does NOT call the sandbox adapter to
// tear anything down. Actual sandbox teardown relies on Sandbox SDK's
// own idle timeout per SPEC-100 §5.3 (UNCONFIRMED — verify against
// current Sandbox SDK docs; add an explicit destroy call here if that
// assumption turns out to be wrong).
func (s *Store) reapLoop() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		s.mu.Lock()
		for id, l := range s.leases {
			if time.Now().After(l.ExpiresAt) {
				delete(s.leases, id)
			}
		}
		s.mu.Unlock()
	}
}
