// Package mcp implements the four tool handlers this server exposes.
// See /docs/SPEC-100.md §5.1 for the tool contracts and §5.2 for how
// payment-required responses MUST be shaped: a normal (200-level) tool
// result with IsError: true and structured JSON in TextContent — NOT a
// raw HTTP 402, which MCP's Streamable HTTP transport does not
// guarantee propagates per-tool-call.
package mcp

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/YOUR_ORG/compute-router/services/core/internal/config"
	"github.com/YOUR_ORG/compute-router/services/core/internal/payment"
	"github.com/YOUR_ORG/compute-router/services/core/internal/sandboxclient"
	"github.com/YOUR_ORG/compute-router/services/core/internal/session"
)

// Deps bundles everything a tool handler needs. Built once in main.go.
type Deps struct {
	Cfg      *config.Config
	Sessions *session.Store
	Pay      *payment.Client
	Sandbox  *sandboxclient.Client
}

func newSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func paymentRequiredResult(reqs payment.PaymentRequirements) (*gomcp.CallToolResult, error) {
	payload, err := json.Marshal(map[string]any{
		"error":                "payment_required",
		"payment_requirements": reqs,
	})
	if err != nil {
		return nil, err
	}
	return &gomcp.CallToolResult{
		IsError: true,
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(payload)}},
	}, nil
}

func (d *Deps) buildRequirements(priceUSD float64) payment.PaymentRequirements {
	return payment.PaymentRequirements{
		Scheme:  "exact",
		Network: d.Cfg.CeloChainCAIP2,
		PayTo:   d.Cfg.PayoutWallet,
		Price: payment.Price{
			Amount: payment.DollarsToUSDCBaseUnits(priceUSD),
			Asset:  d.Cfg.USDCTokenAddress,
		},
	}
}

// --- provision_env ---

type ProvisionEnvInput struct {
	Template   string  `json:"template" jsonschema:"preinstalled toolchain — MVP supports only \"node-build\", see SPEC-100 template-selection note"`
	CeilingUSD float64 `json:"ceiling_usd" jsonschema:"max USDC to authorize for this session, e.g. 0.50"`
	PaymentData string `json:"payment_data,omitempty" jsonschema:"signed x402 payment payload — omit on first call, the tool will report payment_required, sign, and retry with this set"`
}

type ProvisionEnvOutput struct {
	SessionID string `json:"session_id"`
	ExpiresAt string `json:"expires_at"`
}

func (d *Deps) ProvisionEnv(ctx context.Context, req *gomcp.CallToolRequest, in ProvisionEnvInput) (*gomcp.CallToolResult, ProvisionEnvOutput, error) {
	priceUSD, _ := parsePrice(d.Cfg.PricePerBlockUSD) // MVP: one block, priced at provision time
	reqs := d.buildRequirements(priceUSD)

	if in.PaymentData == "" {
		result, err := paymentRequiredResult(reqs)
		return result, ProvisionEnvOutput{}, err
	}

	ok, err := d.Pay.Verify(in.PaymentData, reqs)
	if err != nil || !ok {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("payment verification failed: %w", err)
	}
	settled, _, err := d.Pay.Settle(in.PaymentData, reqs)
	if err != nil || !settled {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("payment settlement failed: %w", err)
	}

	sessionID := newSessionID()
	blockMinutes, _ := parsePrice(d.Cfg.BlockMinutes)
	expiresAt := time.Now().Add(time.Duration(blockMinutes) * time.Minute)

	if err := d.Sandbox.Boot(sessionID, in.Template); err != nil {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("sandbox boot failed: %w", err)
	}

	d.Sessions.Put(&session.Lease{
		ID:              sessionID,
		Template:        in.Template,
		ExpiresAt:       expiresAt,
		CeilingUSD:      in.CeilingUSD,
		SettledSoFarUSD: priceUSD,
	})

	return nil, ProvisionEnvOutput{
		SessionID: sessionID,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}, nil
}

// --- exec ---

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
		return nil, ExecOutput{}, err // expired/unknown session — a real error, not a payment gate
	}
	result, err := d.Sandbox.Exec(in.SessionID, in.Command)
	if err != nil {
		return nil, ExecOutput{}, fmt.Errorf("exec failed: %w", err)
	}
	return nil, ExecOutput{Stdout: result.Stdout, Stderr: result.Stderr, ExitCode: result.ExitCode}, nil
}

// --- extend_ceiling ---

type ExtendCeilingInput struct {
	SessionID     string  `json:"session_id"`
	AdditionalUSD float64 `json:"additional_usd"`
	PaymentData   string  `json:"payment_data,omitempty"`
}

type ExtendCeilingOutput struct {
	NewCeilingUSD float64 `json:"new_ceiling_usd"`
	ExpiresAt     string  `json:"expires_at"`
}

func (d *Deps) ExtendCeiling(ctx context.Context, req *gomcp.CallToolRequest, in ExtendCeilingInput) (*gomcp.CallToolResult, ExtendCeilingOutput, error) {
	lease, err := d.Sessions.Get(in.SessionID)
	if err != nil {
		return nil, ExtendCeilingOutput{}, err
	}

	reqs := d.buildRequirements(in.AdditionalUSD)
	if in.PaymentData == "" {
		result, err := paymentRequiredResult(reqs)
		return result, ExtendCeilingOutput{}, err
	}

	ok, err := d.Pay.Verify(in.PaymentData, reqs)
	if err != nil || !ok {
		return nil, ExtendCeilingOutput{}, fmt.Errorf("payment verification failed: %w", err)
	}
	settled, _, err := d.Pay.Settle(in.PaymentData, reqs)
	if err != nil || !settled {
		return nil, ExtendCeilingOutput{}, fmt.Errorf("payment settlement failed: %w", err)
	}

	blockMinutes, _ := parsePrice(d.Cfg.BlockMinutes)
	newExpiry := lease.ExpiresAt.Add(time.Duration(blockMinutes) * time.Minute)
	if err := d.Sessions.Extend(in.SessionID, in.AdditionalUSD, newExpiry); err != nil {
		return nil, ExtendCeilingOutput{}, err
	}

	return nil, ExtendCeilingOutput{
		NewCeilingUSD: lease.CeilingUSD + in.AdditionalUSD,
		ExpiresAt:     newExpiry.Format(time.RFC3339),
	}, nil
}

// --- release ---

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
	_ = d.Sandbox.Destroy(in.SessionID) // best-effort — idle timeout is the real backstop, SPEC-100 §5.3
	d.Sessions.Delete(in.SessionID)
	return nil, ReleaseOutput{FinalCostUSD: lease.SettledSoFarUSD}, nil
}

func parsePrice(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
