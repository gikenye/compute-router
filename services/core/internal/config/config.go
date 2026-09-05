// Package config loads runtime configuration from the environment.
package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                 string
	CeloNetwork          string
	CeloChainCAIP2       string
	FacilitatorURL       string
	FacilitatorAPIKey    string
	CeloRPCURL           string
	CeloChainID          int64
	USDCTokenAddress     string
	USDTTokenAddress     string
	PayoutWallet         string
	SandboxAdapterURL    string
	SandboxAdapterSecret string
	PricePerBlockUSD     string
	BlockMinutes         string

	AgentName         string
	AgentDescription  string
	AgentImageURI     string
	AgentMCPPublicURL string

	SafetyCheckEnabled       bool
	CencoriAPIBaseURL        string
	CencoriAPIKey            string
	SafetyCheckFailOpen      bool
	RefundLedgerPath         string
	RefundOperatorPrivateKey string
	AttributionTag           string
}

func mustEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("required env var %s is not set (see .env.example)", key)
	}
	return v, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1"
}

func Load() (*Config, error) {
	c := &Config{
		Port:             envOr("PORT", "8080"),
		CeloNetwork:      envOr("CELO_NETWORK", "celo-mainnet"),
		CeloChainCAIP2:   envOr("CELO_CHAIN_CAIP2", "eip155:42220"),
		CeloRPCURL:       envOr("CELO_RPC_URL", "https://forno.celo.org"),
		PricePerBlockUSD: envOr("PRICE_PER_BLOCK_USD", "0.02"),
		BlockMinutes:     envOr("BLOCK_MINUTES", "2"),

		AgentName:         envOr("AGENT_NAME", "Compute Router"),
		AgentDescription:  envOr("AGENT_DESCRIPTION", "Agent-native metered compute sandboxes, paid for in USDC on Celo via x402."),
		AgentImageURI:     envOr("AGENT_IMAGE_URI", ""),
		AgentMCPPublicURL: envOr("AGENT_MCP_PUBLIC_URL", ""),

		SafetyCheckEnabled:       envBool("SAFETY_CHECK_ENABLED", false),
		CencoriAPIBaseURL:        envOr("CENCORI_API_BASE_URL", ""),
		CencoriAPIKey:            envOr("CENCORI_API_KEY", ""),
		SafetyCheckFailOpen:      envBool("SAFETY_CHECK_FAIL_OPEN", true),
		RefundLedgerPath:         envOr("REFUND_LEDGER_PATH", "./data/refund-ledger.jsonl"),
		RefundOperatorPrivateKey: envOr("REFUND_OPERATOR_PRIVATE_KEY", os.Getenv("AGENT_OPERATOR_PRIVATE_KEY")),
		AttributionTag:           envOr("ATTRIBUTION_ASSIGNED_TAG", "celo_9f3a3bce8894"),
	}

	var err error
	if c.FacilitatorURL, err = mustEnv("X402_FACILITATOR_URL"); err != nil {
		return nil, err
	}
	c.CeloChainID = 42220
	if c.FacilitatorAPIKey, err = mustEnv("X402_API_KEY"); err != nil {
		return nil, err
	}
	if c.USDCTokenAddress, err = mustEnv("USDC_TOKEN_ADDRESS"); err != nil {
		return nil, err
	}
	c.USDTTokenAddress = envOr("USDT_TOKEN_ADDRESS", "0x48065fbbe25f71c9282ddf5e1cd6d6a887483d5e")
	if c.PayoutWallet, err = mustEnv("PAYOUT_WALLET"); err != nil {
		return nil, err
	}
	if c.SandboxAdapterURL, err = mustEnv("SANDBOX_ADAPTER_URL"); err != nil {
		return nil, err
	}
	if c.SandboxAdapterSecret, err = mustEnv("SANDBOX_ADAPTER_SHARED_SECRET"); err != nil {
		return nil, err
	}

	if c.SafetyCheckEnabled && (c.CencoriAPIBaseURL == "" || c.CencoriAPIKey == "") {
		return nil, fmt.Errorf("SAFETY_CHECK_ENABLED=true requires CENCORI_API_BASE_URL and CENCORI_API_KEY")
	}

	return c, nil
}
