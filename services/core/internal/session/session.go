// Package session tracks active lease state, with optional file-backed persistence.
package session

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrKeyInFlight is returned by Reserve when another concurrent request is
// already provisioning with the same idempotency key.
var ErrKeyInFlight = errors.New("idempotency key is currently in-flight; retry after the in-progress request completes")

// Lease records the state of a single provisioned session.
type Lease struct {
	ID               string    `json:"id"`
	Template         string    `json:"template"`
	ExpiresAt        time.Time `json:"expires_at"`
	CeilingUSD       float64   `json:"ceiling_usd"`
	SettledSoFarUSD  float64   `json:"settled_so_far_usd"`
	IdempotencyKey   string    `json:"idempotency_key,omitempty"`
}

// pendingKey records an idempotency key that has been reserved for an
// in-progress provisioning attempt.
type pendingKey struct {
	Template string
}

// Store is an in-memory lease registry with optional JSONL persistence.
// When DatabasePath is non-empty it writes new leases to that file and
// reloads them on startup, so sessions survive process restarts.
type Store struct {
	mu               sync.Mutex
	leases           map[string]*Lease
	idempotencyIndex map[string]*Lease  // idempotency key → committed lease
	pendingKeys      map[string]*pendingKey // idempotency key → in-flight reservation
	DatabasePath     string
	f                *os.File
}

// NewStore returns a purely in-memory store.
func NewStore() *Store {
	s := &Store{
		leases:           make(map[string]*Lease),
		idempotencyIndex: make(map[string]*Lease),
		pendingKeys:      make(map[string]*pendingKey),
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
		pendingKeys:      make(map[string]*pendingKey),
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

// leaseRecord is the JSONL on-disk shape. Deleted is set to true for tombstones.
type leaseRecord struct {
	Lease
	Deleted bool `json:"deleted,omitempty"`
}

// load reads the JSONL file and interns non-expired, non-deleted leases.
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
		var rec leaseRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue // skip corrupt lines
		}
		if rec.Deleted {
			// Tombstone: remove any previously loaded entry with this ID.
			if existing, ok := s.leases[rec.ID]; ok && existing.IdempotencyKey != "" {
				delete(s.idempotencyIndex, existing.IdempotencyKey)
			}
			delete(s.leases, rec.ID)
			continue
		}
		if rec.ExpiresAt.After(now) {
			l := rec.Lease
			s.leases[l.ID] = &l
			if l.IdempotencyKey != "" {
				s.idempotencyIndex[l.IdempotencyKey] = &l
			}
		}
	}
	return scanner.Err()
}

// appendRecord writes a leaseRecord to the backing file, if one is open.
func (s *Store) appendRecord(rec leaseRecord) {
	if s.f == nil {
		return
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return
	}
	b = append(b, '\n')
	_, _ = s.f.Write(b)
}

// append writes a live lease record to the backing file.
func (s *Store) append(l *Lease) {
	s.appendRecord(leaseRecord{Lease: *l})
}

func (s *Store) Put(l *Lease) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.leases[l.ID] = l
	if l.IdempotencyKey != "" {
		s.idempotencyIndex[l.IdempotencyKey] = l
		delete(s.pendingKeys, l.IdempotencyKey) // fulfil the reservation
	}
	s.append(l)
}

// Reserve atomically looks up or claims an idempotency key.
//
//   - Returns (existing lease, nil) when a non-expired lease already exists for
//     the key; the caller should return the original session to the client.
//     Returns an error if the existing lease used a different template.
//   - Returns (nil, nil) when the key was successfully reserved; the caller must
//     call Put on success or ReleaseReservation on any failure.
//   - Returns (nil, ErrKeyInFlight) when another request is currently provisioning
//     with this key; the caller should surface a retriable conflict to the client.
func (s *Store) Reserve(key, template string) (*Lease, error) {
	if key == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	// Active lease: return it if the template matches.
	if l, ok := s.idempotencyIndex[key]; ok {
		if time.Now().After(l.ExpiresAt) {
			// Expired: evict and fall through to reserve.
			if l.IdempotencyKey != "" {
				delete(s.idempotencyIndex, l.IdempotencyKey)
			}
			delete(s.leases, l.ID)
		} else {
			if l.Template != template {
				return nil, fmt.Errorf("idempotency key %q was used with template %q; cannot reuse for template %q", key, l.Template, template)
			}
			return l, nil
		}
	}

	// In-flight: another concurrent request already holds this key.
	if _, ok := s.pendingKeys[key]; ok {
		return nil, ErrKeyInFlight
	}

	// Claim the key.
	s.pendingKeys[key] = &pendingKey{Template: template}
	return nil, nil
}

// ReleaseReservation removes a pending key reservation created by Reserve.
// Safe to call when the key is not pending (no-op), including after Put.
func (s *Store) ReleaseReservation(key string) {
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.pendingKeys, key)
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
	if l, ok := s.leases[id]; ok {
		if l.IdempotencyKey != "" {
			delete(s.idempotencyIndex, l.IdempotencyKey)
		}
		// Write a tombstone so load() skips this lease on restart.
		s.appendRecord(leaseRecord{Lease: *l, Deleted: true})
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
