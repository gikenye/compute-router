// Package config loads runtime configuration from the environment.
// Nothing here has defaults for secrets — missing required values fail
// fast at startup rather than silently running unconfigured.
package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                  string
	CeloNetwork            string
	CeloChainCAIP2         string
	FacilitatorURL         string
	FacilitatorAPIKey      string
	USDCTokenAddress       string
	PayoutWallet           string
	SandboxAdapterURL      string
	SandboxAdapterSecret   string
	PricePerBlockUSD       string // kept as string; converted to base units at the payment boundary
	BlockMinutes           string
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

// Load reads and validates configuration. It intentionally does NOT
// default facilitator/payout/adapter secrets — those must be explicit.
func Load() (*Config, error) {
	c := &Config{
		Port:             envOr("PORT", "8080"),
		CeloNetwork:      envOr("CELO_NETWORK", "celo-sepolia"),
		CeloChainCAIP2:   envOr("CELO_CHAIN_CAIP2", "eip155:11142220"),
		PricePerBlockUSD: envOr("PRICE_PER_BLOCK_USD", "0.02"),
		BlockMinutes:     envOr("BLOCK_MINUTES", "2"),
	}

	var err error
	if c.FacilitatorURL, err = mustEnv("X402_FACILITATOR_URL"); err != nil {
		return nil, err
	}
	if c.FacilitatorAPIKey, err = mustEnv("X402_API_KEY"); err != nil {
		return nil, err
	}
	if c.USDCTokenAddress, err = mustEnv("USDC_TOKEN_ADDRESS"); err != nil {
		return nil, err
	}
	if c.PayoutWallet, err = mustEnv("PAYOUT_WALLET"); err != nil {
		return nil, err
	}
	if c.SandboxAdapterURL, err = mustEnv("SANDBOX_ADAPTER_URL"); err != nil {
		return nil, err
	}
	if c.SandboxAdapterSecret, err = mustEnv("SANDBOX_ADAPTER_SHARED_SECRET"); err != nil {
		return nil, err
	}
	return c, nil
}
