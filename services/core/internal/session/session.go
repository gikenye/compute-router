// Package session tracks active lease state in memory.
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
	mu     sync.Mutex
	leases map[string]*Lease
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
