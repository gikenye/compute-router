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
	// Sandboxes maps template name to adapter client. "node-build" is always
	// present; "python-build" and "rust-build" are added when their adapter
	// URLs are configured. adapterFor() selects the right client by template.
	Sandboxes map[string]*sandboxclient.Client
	Safety    *safety.Checker
}

// adapterFor returns the sandbox adapter for the given template name, or an
// error if the template is unknown or its adapter is not configured.
func (d *Deps) adapterFor(template string) (*sandboxclient.Client, error) {
	c, ok := d.Sandboxes[template]
	if !ok || c == nil {
		known := make([]string, 0, len(d.Sandboxes))
		for k := range d.Sandboxes {
			known = append(known, k)
		}
		return nil, fmt.Errorf("unknown or unconfigured template %q (available: %v)", template, known)
	}
	return c, nil
}

// KnownTemplates returns the sorted list of templates this deployment supports.
func (d *Deps) KnownTemplates() []string {
	templates := make([]string, 0, len(d.Sandboxes))
	for k := range d.Sandboxes {
		templates = append(templates, k)
	}
	return templates
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
	opts := []payment.PaymentRequirements{
		d.buildRequirementsFor(priceUSD, "USDC"),
		d.buildRequirementsFor(priceUSD, "USDT"),
	}
	// LatAm Permit2 assets are offered only when their token addresses are configured.
	if d.Cfg.WARSTokenAddress != "" {
		opts = append(opts, d.buildRequirementsFor(priceUSD, "wARS"))
	}
	if d.Cfg.WBRLTokenAddress != "" {
		opts = append(opts, d.buildRequirementsFor(priceUSD, "wBRL"))
	}
	if d.Cfg.WCOPTokenAddress != "" {
		opts = append(opts, d.buildRequirementsFor(priceUSD, "wCOP"))
	}
	return opts
}

// isKnownAsset reports whether symbol is a configured, payable asset.
// LatAm assets (wARS, wBRL, wCOP) are only available when their token
// addresses are set in config.
func (d *Deps) isKnownAsset(symbol string) bool {
	switch symbol {
	case "USDC", "USDT":
		return true
	case "wARS":
		return d.Cfg.WARSTokenAddress != ""
	case "wBRL":
		return d.Cfg.WBRLTokenAddress != ""
	case "wCOP":
		return d.Cfg.WCOPTokenAddress != ""
	default:
		return false
	}
}

func (d *Deps) buildRequirementsFor(priceUSD float64, symbol string) payment.PaymentRequirements {
	base := payment.PaymentRequirements{
		Scheme:  "exact",
		Network: d.Cfg.CeloChainCAIP2,
		PayTo:   d.Cfg.PayoutWallet,
	}
	switch symbol {
	case "USDT":
		base.Price = payment.Price{
			Amount: payment.DollarsToBaseUnits(priceUSD, 6),
			Asset:  d.Cfg.USDTTokenAddress,
			Extra:  map[string]string{"name": "Tether USD", "version": "1"},
		}
	case "wARS":
		// Hardcoded FX rate for MVP; replaced with live Tipo rates in a later phase.
		const arsPerUSD = 1050.0
		base.Price = payment.Price{
			Amount: payment.FXToBaseUnits(priceUSD, 18, arsPerUSD),
			Asset:  d.Cfg.WARSTokenAddress,
			Extra:  map[string]string{"name": "Peso Argentino", "version": "1", "assetTransferMethod": "permit2"},
		}
	case "wBRL":
		const brlPerUSD = 5.8
		base.Price = payment.Price{
			Amount: payment.FXToBaseUnits(priceUSD, 18, brlPerUSD),
			Asset:  d.Cfg.WBRLTokenAddress,
			Extra:  map[string]string{"name": "Real Brasileiro", "version": "1", "assetTransferMethod": "permit2"},
		}
	case "wCOP":
		const copPerUSD = 4100.0
		base.Price = payment.Price{
			Amount: payment.FXToBaseUnits(priceUSD, 18, copPerUSD),
			Asset:  d.Cfg.WCOPTokenAddress,
			Extra:  map[string]string{"name": "Peso Colombiano", "version": "1", "assetTransferMethod": "permit2"},
		}
	default: // USDC
		base.Price = payment.Price{
			Amount: payment.DollarsToBaseUnits(priceUSD, 6),
			Asset:  d.Cfg.USDCTokenAddress,
			Extra:  map[string]string{"name": "USDC", "version": "2"},
		}
	}
	return base
}

type ProvisionEnvInput struct {
	Template        string  `json:"template" jsonschema:"preinstalled toolchain; node-build is always available; python-build and rust-build when configured"`
	CeilingUSD      float64 `json:"ceiling_usd" jsonschema:"max USDC to authorize for this session, e.g. 0.50"`
	PaymentAsset    string  `json:"payment_asset,omitempty" jsonschema:"stablecoin to use: USDC, USDT, wARS, wBRL, or wCOP; defaults to USDC; LatAm assets (wARS/wBRL/wCOP) use Permit2 and are offered when configured"`
	PaymentData     string  `json:"payment_data,omitempty" jsonschema:"signed x402 payment payload - omit on first call, the tool will report payment_required, sign, and retry with this set"`
	IdempotencyKey  string  `json:"idempotency_key,omitempty" jsonschema:"caller-supplied unique key; a duplicate provision_env with the same key returns the original session instead of creating a new one"`
}

type ProvisionEnvOutput struct {
	SessionID        string `json:"session_id"`
	ExpiresAt        string `json:"expires_at"`
	SettlementTxHash string `json:"settlement_tx_hash,omitempty"`
}

func (d *Deps) ProvisionEnv(ctx context.Context, req *gomcp.CallToolRequest, in ProvisionEnvInput) (*gomcp.CallToolResult, ProvisionEnvOutput, error) {
	// Idempotency: if the caller supplied a key and a matching non-expired session
	// already exists, return the original session without a new payment.
	if in.IdempotencyKey != "" {
		if existing := d.Sessions.GetByIdempotencyKey(in.IdempotencyKey); existing != nil {
			return nil, ProvisionEnvOutput{
				SessionID: existing.ID,
				ExpiresAt: existing.ExpiresAt.Format(time.RFC3339),
			}, nil
		}
	}

	priceUSD, _ := parsePrice(d.Cfg.PricePerBlockUSD)
	paymentAsset := in.PaymentAsset
	if paymentAsset == "" {
		paymentAsset = "USDC"
	}
	if !d.isKnownAsset(paymentAsset) {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("unsupported payment_asset %q: accepted assets are USDC, USDT, wARS, wBRL, wCOP (LatAm assets offered only when configured)", paymentAsset)
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

	adapter, err := d.adapterFor(in.Template)
	if err != nil {
		return nil, ProvisionEnvOutput{}, err
	}

	if err := adapter.Boot(sessionID, in.Template); err != nil {
		return nil, ProvisionEnvOutput{}, fmt.Errorf("sandbox boot failed before settlement; no payment was submitted: %w", err)
	}
	settled, txHash, err := d.Pay.Settle(in.PaymentData, reqs)
	if err != nil || !settled {
		// Bug fix: log destroy errors instead of silently discarding them.
		if destroyErr := adapter.Destroy(sessionID); destroyErr != nil {
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
		IdempotencyKey:  in.IdempotencyKey,
	})

	return nil, ProvisionEnvOutput{
		SessionID:        sessionID,
		ExpiresAt:        expiresAt.Format(time.RFC3339),
		SettlementTxHash: txHash,
	}, nil
}

// defaultMaxOutputBytes caps output per stream to avoid memory exhaustion.
const defaultMaxOutputBytes = 256 * 1024 // 256 KiB

type ExecInput struct {
	SessionID      string `json:"session_id"`
	Command        string `json:"command"`
	// MaxOutputBytes caps stdout and stderr independently (default 262144 = 256 KiB).
	// Output that exceeds the cap is truncated and a truncation notice appended.
	MaxOutputBytes int    `json:"max_output_bytes,omitempty" jsonschema:"max bytes per output stream; default 262144"`
}

type ExecOutput struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exit_code"`
	Truncated bool   `json:"truncated,omitempty"` // true when either stream was capped
}

func (d *Deps) Exec(ctx context.Context, req *gomcp.CallToolRequest, in ExecInput) (*gomcp.CallToolResult, ExecOutput, error) {
	lease, err := d.Sessions.Get(in.SessionID)
	if err != nil {
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

	adapter, err := d.adapterFor(lease.Template)
	if err != nil {
		return nil, ExecOutput{}, fmt.Errorf("exec routing error: %w", err)
	}
	result, err := adapter.Exec(in.SessionID, in.Command)
	if err != nil {
		return nil, ExecOutput{}, fmt.Errorf("exec failed: %w", err)
	}

	capBytes := in.MaxOutputBytes
	if capBytes <= 0 {
		capBytes = defaultMaxOutputBytes
	}
	stdout, stderrStr, truncated := capOutputs(result.Stdout, result.Stderr, capBytes)
	return nil, ExecOutput{
		Stdout:    stdout,
		Stderr:    stderrStr,
		ExitCode:  result.ExitCode,
		Truncated: truncated,
	}, nil
}

// capOutputs truncates each stream independently to maxBytes, appending a
// notice so callers know output was elided.
func capOutputs(stdout, stderr string, maxBytes int) (string, string, bool) {
	truncated := false
	if len(stdout) > maxBytes {
		stdout = stdout[:maxBytes] + "\n[output truncated]"
		truncated = true
	}
	if len(stderr) > maxBytes {
		stderr = stderr[:maxBytes] + "\n[output truncated]"
		truncated = true
	}
	return stdout, stderr, truncated
}

type ExtendCeilingInput struct {
	SessionID     string  `json:"session_id"`
	AdditionalUSD float64 `json:"additional_usd"`
	PaymentAsset  string  `json:"payment_asset,omitempty" jsonschema:"stablecoin to use: USDC, USDT, wARS, wBRL, or wCOP; defaults to USDC; LatAm assets (wARS/wBRL/wCOP) use Permit2 and are offered when configured"`
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
	if !d.isKnownAsset(paymentAsset) {
		return nil, ExtendCeilingOutput{}, fmt.Errorf("unsupported payment_asset %q: accepted assets are USDC, USDT, wARS, wBRL, wCOP (LatAm assets offered only when configured)", paymentAsset)
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
	// Route destroy to the adapter that owns this session's template.
	if adapter, adapterErr := d.adapterFor(lease.Template); adapterErr == nil {
		if destroyErr := adapter.Destroy(in.SessionID); destroyErr != nil {
			log.Printf("warning: sandbox destroy failed for session %s: %v", in.SessionID, destroyErr)
		}
	} else {
		log.Printf("warning: could not find adapter for template %q during release of session %s: %v", lease.Template, in.SessionID, adapterErr)
	}
	d.Sessions.Delete(in.SessionID)
	return nil, ReleaseOutput{FinalCostUSD: lease.SettledSoFarUSD}, nil
}

func parsePrice(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}
