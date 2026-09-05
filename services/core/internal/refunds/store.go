package refunds

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	s.mu.Lock()
	idSum := sha256.Sum256([]byte(paymentData + asset + amount))
	id := "rf_" + hex.EncodeToString(idSum[:8])
	if existing, ok := s.findLocked(id); ok {
		s.mu.Unlock()
		return existing, nil
	}
	s.mu.Unlock()
	payer := payerFromPayment(paymentData)
	txHash, err := s.sender.Transfer(ctx, asset, payer, amount, s.tag)
	if err != nil {
		return Record{}, err
	}
	return s.record(paymentData, asset, amount, reason, txHash, "refunded")
}

func (s *Store) findLocked(id string) (Record, bool) {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		return Record{}, false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		var record Record
		if json.Unmarshal([]byte(line), &record) == nil && record.ID == id {
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
