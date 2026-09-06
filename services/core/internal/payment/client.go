// Package payment implements the x402 client for the Celo facilitator.
package payment

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Price struct {
	Amount string            `json:"amount"`
	Asset  string            `json:"asset"`
	Extra  map[string]string `json:"extra,omitempty"`
}

type Stablecoin struct {
	Symbol   string
	Address  string
	Decimals int
	Name     string
	Version  string
}

type PaymentRequirements struct {
	Scheme  string `json:"scheme"`
	Network string `json:"network"`
	PayTo   string `json:"payTo"`
	Price   Price  `json:"price"`
}

// NegotiationFormat returns the x402 v2 PaymentRequirements shape exposed to
// clients. The nested Price form remains the internal service representation.
func (r PaymentRequirements) NegotiationFormat() map[string]any {
	return map[string]any{
		"scheme":            r.Scheme,
		"network":           r.Network,
		"asset":             r.Price.Asset,
		"payTo":             r.PayTo,
		"amount":            r.Price.Amount,
		"maxTimeoutSeconds": 300,
		"extra":             r.Price.Extra,
	}
}

type facilitatorRequirements struct {
	Scheme            string            `json:"scheme"`
	Network           string            `json:"network"`
	Asset             string            `json:"asset"`
	PayTo             string            `json:"payTo"`
	Amount            string            `json:"amount"`
	MaxTimeoutSeconds int               `json:"maxTimeoutSeconds"`
	Extra             map[string]string `json:"extra,omitempty"`
}

func (r PaymentRequirements) facilitatorFormat() facilitatorRequirements {
	return facilitatorRequirements{
		Scheme:            r.Scheme,
		Network:           r.Network,
		Asset:             r.Price.Asset,
		PayTo:             r.PayTo,
		Amount:            r.Price.Amount,
		MaxTimeoutSeconds: 300,
		Extra:             r.Price.Extra,
	}
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

func DollarsToUSDCBaseUnits(dollars float64) string {
	return DollarsToBaseUnits(dollars, 6)
}

func DollarsToBaseUnits(dollars float64, decimals int) string {
	multiplier := 1.0
	for i := 0; i < decimals; i++ {
		multiplier *= 10
	}
	return fmt.Sprintf("%d", int64(dollars*multiplier))
}

type verifyResponse struct {
	Valid        bool   `json:"isValid"`
	Error        string `json:"invalidReason,omitempty"`
	ErrorDetails string `json:"invalidReasonDetails,omitempty"`
}

func (c *Client) Verify(paymentData string, req PaymentRequirements) (bool, error) {
	payload, err := decodePayment(paymentData)
	if err != nil {
		return false, err
	}
	body, err := json.Marshal(map[string]any{
		"x402Version":         2,
		"paymentPayload":      payload,
		"paymentRequirements": req.facilitatorFormat(),
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
		return false, fmt.Errorf("payment invalid: %s (%s)", vr.Error, vr.ErrorDetails)
	}
	return true, nil
}

type settleResponse struct {
	Success     bool   `json:"success"`
	Transaction string `json:"transaction,omitempty"`
	Error       string `json:"error,omitempty"`
}

// SettlementError carries the outcome of a failed settlement.
//
// Confirmed is true only when the facilitator provides evidence that the
// transaction was not submitted to the network (e.g. "invalid_payment",
// "insufficient_funds").  It is false for transient or ambiguous outcomes
// such as "settlement_pending", network timeouts, or unknown error strings,
// so callers can route those to manual review rather than auto-refund.
type SettlementError struct {
	Confirmed bool   // true => facilitator confirmed no value captured; safe to refund
	TxHash    string // non-empty when the facilitator reported a transaction hash
	Err       error
}

func (e *SettlementError) Error() string { return e.Err.Error() }
func (e *SettlementError) Unwrap() error { return e.Err }

// finalNonSubmissionErrors is the set of facilitator error reasons that
// conclusively prove the payment was never submitted on-chain.  All other
// error strings (including "settlement_pending") are treated as ambiguous.
var finalNonSubmissionErrors = map[string]bool{
	"invalid_payment":    true,
	"insufficient_funds": true,
	"invalid_scheme":     true,
	"invalid_network":    true,
	"invalid_asset":      true,
	"invalid_amount":     true,
	"payment_expired":    true,
	"already_settled":    true, // duplicate — value was captured; route to review
}

func (c *Client) Settle(paymentData string, req PaymentRequirements) (settled bool, txHash string, err error) {
	payload, err := decodePayment(paymentData)
	if err != nil {
		return false, "", err
	}
	body, err := json.Marshal(map[string]any{
		"x402Version":         2,
		"paymentPayload":      payload,
		"paymentRequirements": req.facilitatorFormat(),
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
	if !sr.Success {
		// Preserve a non-empty transaction hash so callers can record it for review.
		reportedTx := sr.Transaction

		// "settlement_pending" and any unrecognised reason are ambiguous: the
		// transaction may be in-flight.  Only set Confirmed=true for reasons
		// that prove the payment was never submitted to the chain.
		confirmed := finalNonSubmissionErrors[sr.Error]

		return false, reportedTx, &SettlementError{
			Confirmed: confirmed,
			TxHash:    reportedTx,
			Err:       fmt.Errorf("settlement failed: %s", sr.Error),
		}
	}
	return true, sr.Transaction, nil
}

func decodePayment(paymentData string) (map[string]any, error) {
	raw, err := base64.StdEncoding.DecodeString(paymentData)
	if err != nil {
		return nil, fmt.Errorf("payment_data is not valid base64: %w", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("payment_data is not valid JSON: %w", err)
	}
	return payload, nil
}
