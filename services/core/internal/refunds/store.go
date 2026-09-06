package refunds

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

type Record struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Reason    string    `json:"reason"`
	Payer     string    `json:"payer,omitempty"`
	Asset     string    `json:"asset"`
	Amount    string    `json:"amount"`
	TxHash    string    `json:"tx_hash,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	mu     sync.Mutex
	path   string
	sender *Sender
	tag    string
}

func New(path string, sender *Sender, tag string) *Store {
	return &Store{path: path, sender: sender, tag: tag}
}

func (s *Store) RecordReview(paymentData string, asset, amount, reason, txHash string) (Record, error) {
	return s.record(paymentData, asset, amount, reason, txHash, "review_required")
}

func (s *Store) record(paymentData string, asset, amount, reason, txHash, status string) (Record, error) {
	payer := ""
	var envelope struct {
		Payload struct {
			Authorization struct {
				From string `json:"from"`
			} `json:"authorization"`
		} `json:"payload"`
	}

	if raw, err := base64.StdEncoding.DecodeString(paymentData); err == nil {
		if err := json.Unmarshal(raw, &envelope); err == nil {
			payer = envelope.Payload.Authorization.From
		}
	}
	now := time.Now().UTC()
	sum := sha256.Sum256([]byte(paymentData + asset + amount))
	record := Record{
		ID:        "rf_" + hex.EncodeToString(sum[:8]),
		Status:    status,
		Reason:    reason,
		Payer:     payer,
		Asset:     asset,
		Amount:    amount,
		TxHash:    txHash,
		CreatedAt: now,
	}
	line, err := json.Marshal(record)
	if err != nil {
		return Record{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepathDir(s.path), 0o750); err != nil {
		return Record{}, fmt.Errorf("create refund ledger directory: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Record{}, fmt.Errorf("open refund ledger: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return Record{}, fmt.Errorf("write refund ledger: %w", err)
	}
	return record, nil
}

func (s *Store) Refund(ctx context.Context, paymentData, asset, amount, reason string) (Record, error) {
	// Derive a stable idempotency ID from payment identity.
	idSum := sha256.Sum256([]byte(paymentData + asset + amount))
	id := "rf_" + hex.EncodeToString(idSum[:8])

	// Hold the mutex while checking for an existing reservation and, if none
	// exists, writing a pending record.  This makes the reservation atomic:
	// a concurrent or retried call finds the pending entry and returns it
	// rather than launching a second transfer.
	s.mu.Lock()
	if existing, ok := s.findLocked(id); ok {
		s.mu.Unlock()
		if existing.TxHash != "" && (existing.Status == "unknown" || existing.Status == "submitted") {
			return s.reconcile(ctx, existing)
		}
		return existing, nil
	}
	payer := payerFromPayment(paymentData)
	pendingRecord, pendingErr := s.writeLocked(Record{
		ID:        id,
		Status:    "pending",
		Reason:    reason,
		Payer:     payer,
		Asset:     asset,
		Amount:    amount,
		CreatedAt: time.Now().UTC(),
	})
	s.mu.Unlock()

	if pendingErr != nil {
		// Could not persist the reservation; bail without transferring to
		// avoid an un-trackable refund.
		return Record{}, fmt.Errorf("reserve pending refund record: %w", pendingErr)
	}
	_ = pendingRecord

	txHash, transferErr := s.sender.Transfer(ctx, asset, payer, amount, s.tag)
	if transferErr != nil {
		status := "failed"
		if txHash != "" {
			status = "unknown"
			var typedErr *TransferError
			if errors.As(transferErr, &typedErr) && typedErr.Confirmed {
				status = "failed"
			}
		}
		if _, statusErr := s.updateStatus(id, status, txHash, transferErr.Error()); statusErr != nil {
			return pendingRecord, fmt.Errorf("refund transfer failed and ledger update failed: %v; ledger error: %w", transferErr, statusErr)
		}
		if txHash != "" {
			pendingRecord.TxHash = txHash
			return pendingRecord, fmt.Errorf("refund transaction %s requires reconciliation: %w", txHash, transferErr)
		}

		return Record{}, transferErr
	}

	// Reconcile the reservation to submitted with the confirmed tx hash.
	refunded, statusErr := s.updateStatus(id, "refunded", txHash, "")
	if statusErr != nil {
		pendingRecord.TxHash = txHash
		return pendingRecord, fmt.Errorf("refund submitted as %s but ledger update failed: %w", txHash, statusErr)
	}
	return refunded, nil
}

func (s *Store) reconcile(ctx context.Context, record Record) (Record, error) {
	known, successful, err := s.sender.Reconcile(ctx, record.TxHash)
	if err != nil {
		return record, fmt.Errorf("reconcile refund transaction %s: %w", record.TxHash, err)
	}
	if !known {
		return record, nil
	}
	status := "failed"
	if successful {
		status = "refunded"
	}
	return s.updateStatus(record.ID, status, record.TxHash, "")
}

// writeLocked appends r to the ledger. Caller must hold s.mu.
func (s *Store) writeLocked(r Record) (Record, error) {
	line, err := json.Marshal(r)
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(filepathDir(s.path), 0o750); err != nil {
		return Record{}, fmt.Errorf("create refund ledger directory: %w", err)
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return Record{}, fmt.Errorf("open refund ledger: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return Record{}, fmt.Errorf("write refund ledger: %w", err)
	}
	if err := f.Sync(); err != nil {
		return Record{}, fmt.Errorf("sync refund ledger: %w", err)
	}
	if err := syncDirectory(filepathDir(s.path)); err != nil {
		return Record{}, fmt.Errorf("sync refund ledger directory: %w", err)
	}
	return r, nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// updateStatus appends an updated copy of the record identified by id.
// Readers use the last entry with a matching ID, so appending is safe.
func (s *Store) updateStatus(id, status, txHash, errMsg string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	existing, ok := s.findLocked(id)
	if !ok {
		return Record{}, fmt.Errorf("refund record %s not found for status update", id)
	}
	existing.Status = status
	if txHash != "" {
		existing.TxHash = txHash
	}
	if errMsg != "" {
		existing.Reason = existing.Reason + "; " + errMsg
	}
	return s.writeLocked(existing)
}

func (s *Store) findLocked(id string) (Record, bool) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return Record{}, false
	}
	lines := strings.Split(string(raw), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		var record Record
		if json.Unmarshal([]byte(lines[i]), &record) == nil && record.ID == id {
			return record, true
		}
	}
	return Record{}, false
}

func payerFromPayment(paymentData string) string {
	raw, err := base64.StdEncoding.DecodeString(paymentData)
	if err != nil {
		return ""
	}
	var envelope struct {
		Payload struct {
			Authorization struct {
				From string `json:"from"`
			} `json:"authorization"`
		} `json:"payload"`
	}
	if json.Unmarshal(raw, &envelope) != nil {
		return ""
	}
	return envelope.Payload.Authorization.From
}

func filepathDir(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			if i == 0 {
				return path[:1]
			}
			return path[:i]
		}
	}
	return "."
}
