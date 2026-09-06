// Package mcp implements the server tool handlers.
package mcp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gikenye/compute-router/services/core/internal/config"
	"github.com/gikenye/compute-router/services/core/internal/payment"
	"github.com/gikenye/compute-router/services/core/internal/refunds"
	"github.com/gikenye/compute-router/services/core/internal/safety"
	"github.com/gikenye/compute-router/services/core/internal/sandboxclient"
	"github.com/gikenye/compute-router/services/core/internal/session"
)

// Deps bundles everything a tool handler needs. Built once in main.go.
type Deps struct {
	Cfg      *config.Config
	Sessions *session.Store
	Pay      *payment.Client
	Refunds  *refunds.Store
	Sandbox  *sandboxclient.Client
	Safety   *safety.Checker
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func paymentRequiredResult(reqs []payment.PaymentRequirements) (*gomcp.CallToolResult, error) {
	accepts := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		accepts = append(accepts, req.NegotiationFormat())
	}
	payload, err := json.Marshal(map[string]any{
		"error":       "payment_required",
		"x402Version": 2,
		"accepts":     accepts,
	})
	if err != nil {
		return nil, err
	}
	return &gomcp.CallToolResult{
		IsError: true,
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(payload)}},
	}, nil
}

func (d *Deps) buildPaymentOptions(priceUSD float64) []payment.PaymentRequirements {
	return []payment.PaymentRequirements{
		d.buildRequirementsFor(priceUSD, "USDC"),
		d.buildRequirementsFor(priceUSD, "USDT"),
	}
}

func (d *Deps) buildRequirementsFor(priceUSD float64, symbol string) payment.PaymentRequirements {
	asset := d.Cfg.USDCTokenAddress
	name, version := "USDC", "2"
	decimals := 6
	if symbol == "USDT" {
		asset = d.Cfg.USDTTokenAddress
		name, version = "Tether USD", "1"
	}
	return payment.PaymentRequirements{
		Scheme:  "exact",
		Network: d.Cfg.CeloChainCAIP2,
		PayTo:   d.Cfg.PayoutWallet,
		Price: payment.Price{
			Amount: payment.DollarsToBaseUnits(priceUSD, decimals),
			Asset:  asset,
			Extra:  map[string]string{"name": name, "version": version},
		},
	}
}

type ProvisionEnvInput struct {
	Template     string  `json:"template" jsonschema:"preinstalled toolchain; currently supports only node-build"`
	CeilingUSD   float64 `json:"ceiling_usd" jsonschema:"max USDC to authorize for this session, e.g. 0.50"`
	PaymentAsset string  `json:"payment_asset,omitempty" jsonschema:"stablecoin to use: USDC or USDT; defaults to USDC"`
	PaymentData  string  `json:"payment_data,omitempty" jsonschema:"signed x402 payment payload - omit on first call, the tool will report payment_required, sign, and retry with this set"`
}

type ProvisionEnvOutput struct {
	SessionID        string `json:"session_id"`
	ExpiresAt        string `json:"expires_at"`
	SettlementTxHash string `json:"settlement_tx_hash,omitempty"`
}

func (d *Deps) ProvisionEnv(ctx context.Context, req *gomcp.CallToolRequest, in ProvisionEnvInput) (*gomcp.CallToolResult, ProvisionEnvOutput, error) {
	priceUSD, _ := parsePrice(d.Cfg.PricePerBlockUSD)
	paymentAsset := in.PaymentAsset
	if paymentAsset == "" {
		paymentAsset = "USDC"
	}
	if paymentAsset != "USDC" && paymentAsset != "USDT" {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("unsupported payment_asset %q: use USDC or USDT", paymentAsset)
	}
	reqs := d.buildRequirementsFor(priceUSD, paymentAsset)

	if in.PaymentData == "" {
		result, err := paymentRequiredResult(d.buildPaymentOptions(priceUSD))
		return result, ProvisionEnvOutput{}, err
	}

	ok, err := d.Pay.Verify(in.PaymentData, reqs)
	if err != nil || !ok {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("payment verification failed: %w", err)
	}
	sessionID := newSessionID()
	blockMinutes, _ := parsePrice(d.Cfg.BlockMinutes)
	expiresAt := time.Now().Add(time.Duration(blockMinutes) * time.Minute)

	if err := d.Sandbox.Boot(sessionID, in.Template); err != nil {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("sandbox boot failed before settlement; no payment was submitted: %w", err)
	}
	settled, txHash, err := d.Pay.Settle(in.PaymentData, reqs)
	if err != nil || !settled {
		// Bug fix: log destroy errors instead of silently discarding them.
		if destroyErr := d.Sandbox.Destroy(sessionID); destroyErr != nil {
			log.Printf("warning: sandbox destroy failed for session %s after settlement failure: %v", sessionID, destroyErr)
		}
		var settlementErr *payment.SettlementError
		if errors.As(err, &settlementErr) && settlementErr.Confirmed {
			// Confirmed=true: facilitator proved no value was captured; auto-refund.
			review, ledgerErr := d.Refunds.Refund(ctx, in.PaymentData, reqs.Price.Asset, reqs.Price.Amount, "settlement was rejected after sandbox boot")
			if ledgerErr != nil {
				return nil, ProvisionEnvOutput{}, fmt.Errorf("settlement was rejected and automatic refund failed: %w (original: %v)", ledgerErr, err)
			}
			return nil, ProvisionEnvOutput{}, fmt.Errorf("settlement was rejected; sandbox destroyed; automatic refund %s submitted (tx=%s): %w", review.ID, txHash, err)
		}
		// Confirmed=false (pending/ambiguous): record for manual review, do not auto-refund.
		review, ledgerErr := d.Refunds.RecordReview(in.PaymentData, reqs.Price.Asset, reqs.Price.Amount, "settlement outcome is ambiguous; automatic refund withheld", txHash)
		if ledgerErr != nil {
			return nil, ProvisionEnvOutput{}, fmt.Errorf("settlement outcome is ambiguous and compensation record failed: %w (original: %v)", ledgerErr, err)
		}
		if err == nil {
			err = fmt.Errorf("facilitator did not confirm settlement")
		}
		return nil, ProvisionEnvOutput{}, fmt.Errorf("settlement outcome is ambiguous; sandbox destroyed; refund review %s created (tx=%s): %w", review.ID, txHash, err)
	}

	d.Sessions.Put(&session.Lease{
		ID:              sessionID,
		Template:        in.Template,
		ExpiresAt:       expiresAt,
		CeilingUSD:      in.CeilingUSD,
		SettledSoFarUSD: priceUSD,
	})

	return nil, ProvisionEnvOutput{
		SessionID:        sessionID,
		ExpiresAt:        expiresAt.Format(time.RFC3339),
		SettlementTxHash: txHash,
	}, nil
}

type ExecInput struct {
	SessionID string `json:"session_id"`
	Command   string `json:"command"`
}

type ExecOutput struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func (d *Deps) Exec(ctx context.Context, req *gomcp.CallToolRequest, in ExecInput) (*gomcp.CallToolResult, ExecOutput, error) {
	if _, err := d.Sessions.Get(in.SessionID); err != nil {
		return nil, ExecOutput{}, err
	}

	if allowed, reason, checkErr := d.Safety.Check(in.Command); !allowed {
		denyMsg := fmt.Sprintf("command denied by safety check: %s", reason)
		return &gomcp.CallToolResult{
			IsError: true,
			Content: []gomcp.Content{&gomcp.TextContent{Text: denyMsg}},
		}, ExecOutput{}, nil
	} else if checkErr != nil {
		_ = checkErr
	}

	result, err := d.Sandbox.Exec(in.SessionID, in.Command)
	if err != nil {
		return nil, ExecOutput{}, fmt.Errorf("exec failed: %w", err)
	}
	return nil, ExecOutput{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode}, nil
}

type ExtendCeilingInput struct {
	SessionID     string  `json:"session_id"`
	AdditionalUSD float64 `json:"additional_usd"`
	PaymentAsset  string  `json:"payment_asset,omitempty" jsonschema:"stablecoin to use: USDC or USDT; defaults to USDC"`
	PaymentData   string  `json:"payment_data,omitempty"`
}

type ExtendCeilingOutput struct {
	NewCeilingUSD    float64 `json:"new_ceiling_usd"`
	ExpiresAt        string  `json:"expires_at"`
	SettlementTxHash string  `json:"settlement_tx_hash,omitempty"`
}

func (d *Deps) ExtendCeiling(ctx context.Context, req *gomcp.CallToolRequest, in ExtendCeilingInput) (*gomcp.CallToolResult, ExtendCeilingOutput, error) {
	lease, err := d.Sessions.Get(in.SessionID)
	if err != nil {
		return nil, ExtendCeilingOutput{}, err
	}

	paymentAsset := in.PaymentAsset
	if paymentAsset == "" {
		paymentAsset = "USDC"
	}
	if paymentAsset != "USDC" && paymentAsset != "USDT" {
		return nil, ExtendCeilingOutput{}, fmt.Errorf("unsupported payment_asset %q: use USDC or USDT", paymentAsset)
	}
	reqs := d.buildRequirementsFor(in.AdditionalUSD, paymentAsset)
	if in.PaymentData == "" {
		result, err := paymentRequiredResult(d.buildPaymentOptions(in.AdditionalUSD))
		return result, ExtendCeilingOutput{}, err
	}

	ok, err := d.Pay.Verify(in.PaymentData, reqs)
	if err != nil || !ok {
		return nil, ExtendCeilingOutput{}, fmt.Errorf("payment verification failed: %w", err)
	}
	settled, txHash, err := d.Pay.Settle(in.PaymentData, reqs)
	if err != nil || !settled {
		return nil, ExtendCeilingOutput{}, fmt.Errorf("payment settlement failed: %w", err)
	}

	blockMinutes, _ := parsePrice(d.Cfg.BlockMinutes)
	newExpiry := lease.ExpiresAt.Add(time.Duration(blockMinutes) * time.Minute)
	if err := d.Sessions.Extend(in.SessionID, in.AdditionalUSD, newExpiry); err != nil {
		return nil, ExtendCeilingOutput{}, err
	}

	// Bug fix: Sessions.Extend mutates lease.CeilingUSD += additionalUSD in place.
	// lease.CeilingUSD now holds the updated total; return it directly.
	// Adding in.AdditionalUSD again would double-count the extension.
	return nil, ExtendCeilingOutput{
		NewCeilingUSD:    lease.CeilingUSD,
		ExpiresAt:        newExpiry.Format(time.RFC3339),
		SettlementTxHash: txHash,
	}, nil
}

type ReleaseInput struct {
	SessionID string `json:"session_id"`
}

type ReleaseOutput struct {
	FinalCostUSD float64 `json:"final_cost_usd"`
}

func (d *Deps) Release(ctx context.Context, req *gomcp.CallToolRequest, in ReleaseInput) (*gomcp.CallToolResult, ReleaseOutput, error) {
	lease, err := d.Sessions.Get(in.SessionID)
	if err != nil {
		return nil, ReleaseOutput{}, err
	}
	// Bug fix: log destroy errors instead of silently discarding them.
	if destroyErr := d.Sandbox.Destroy(in.SessionID); destroyErr != nil {
		log.Printf("warning: sandbox destroy failed for session %s: %v", in.SessionID, destroyErr)
	}
	d.Sessions.Delete(in.SessionID)
	return nil, ReleaseOutput{FinalCostUSD: lease.SettledSoFarUSD}, nil
}

func parsePrice(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
