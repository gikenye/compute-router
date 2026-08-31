// Package payment implements the x402 verify/settle client against a
// Celo facilitator. See /docs/SPEC-100.md §4.
//
// [UNCONFIRMED, SPEC-100 §4.2]: PaymentRequirements field names below
// (scheme/network/payTo/price.amount/price.asset/price.extra) were
// reconstructed from docs.celo.org's x402 build guide during design —
// NOT from a full OpenAPI spec. Confirm against
// GET {FACILITATOR_URL}/supported before trusting this in production;
// the struct tags are your single point of correction if the real
// schema differs.
//
// MVP settlement design: fixed-price "exact" scheme, full block price
// settled at provision time (SPEC-100 §4.4 fallback path). The "upto"
// metered-settlement scheme is a stretch goal, not implemented here —
// see the TODO at the bottom of this file.
package payment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Price struct {
	Amount string            `json:"amount"` // smallest units, as a string — e.g. USDC 6 decimals: "20000" = $0.02
	Asset  string            `json:"asset"`  // token contract address
	Extra  map[string]string `json:"extra,omitempty"`
}

type PaymentRequirements struct {
	Scheme  string `json:"scheme"`  // "exact" for MVP
	Network string `json:"network"` // CAIP-2, e.g. "eip155:11142220" for Celo Sepolia
	PayTo   string `json:"payTo"`
	Price   Price  `json:"price"`
}

type Client struct {
	FacilitatorURL string
	APIKey         string
	HTTP           *http.Client
}

func New(facilitatorURL, apiKey string) *Client {
	return &Client{
		FacilitatorURL: facilitatorURL,
		APIKey:         apiKey,
		HTTP:           &http.Client{},
	}
}

// DollarsToUSDCBaseUnits converts a dollar amount to USDC's 6-decimal
// base-unit string. e.g. 0.02 -> "20000".
func DollarsToUSDCBaseUnits(dollars float64) string {
	return fmt.Sprintf("%d", int64(dollars*1_000_000))
}

type verifyResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

// Verify checks a signed payment payload against the facilitator's
// /verify endpoint. Does NOT require the API key per Celo's docs.
func (c *Client) Verify(paymentData string, req PaymentRequirements) (bool, error) {
	body, err := json.Marshal(map[string]any{
		"payment":    paymentData,
		"network":    req.Network,
		"paymentRequirements": req,
	})
	if err != nil {
		return false, err
	}

	httpReq, err := http.NewRequest("POST", c.FacilitatorURL+"/verify", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var vr verifyResponse
	if err := json.Unmarshal(respBody, &vr); err != nil {
		return false, fmt.Errorf("unexpected /verify response shape: %s", string(respBody))
	}
	if !vr.Valid {
		return false, fmt.Errorf("payment invalid: %s", vr.Error)
	}
	return true, nil
}

type settleResponse struct {
	Settled bool   `json:"settled"`
	TxHash  string `json:"txHash,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Settle submits the payment for on-chain settlement. Requires the
// X-API-Key header — this is a server-side secret, never forward it to
// any client-facing response.
func (c *Client) Settle(paymentData string, req PaymentRequirements) (settled bool, txHash string, err error) {
	body, err := json.Marshal(map[string]any{
		"payment":             paymentData,
		"network":             req.Network,
		"paymentRequirements": req,
	})
	if err != nil {
		return false, "", err
	}

	httpReq, err := http.NewRequest("POST", c.FacilitatorURL+"/settle", bytes.NewReader(body))
	if err != nil {
		return false, "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.APIKey)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return false, "", fmt.Errorf("facilitator rejected X-API-Key — check X402_API_KEY")
	}

	var sr settleResponse
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return false, "", fmt.Errorf("unexpected /settle response shape: %s", string(respBody))
	}
	if !sr.Settled {
		return false, "", fmt.Errorf("settlement failed: %s", sr.Error)
	}
	return true, sr.TxHash, nil
}

// TODO (SPEC-100 §4.4, stretch goal): metered "upto" settlement — call
// Settle() multiple times against the same paymentData as usage
// accrues, rather than once at full block price. Requires confirming
// which facilitator actually supports "upto" (Celo's own hosted one
// hedges on this; thirdweb's confirms it) before switching Scheme away
// from "exact" above.
