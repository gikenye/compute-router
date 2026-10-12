// Package agentcard serves the ERC-8004 agent registration file.
package agentcard

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gikenye/compute-router/services/core/internal/config"
)

type Endpoint struct {
	Type    string `json:"type"`
	URL     string `json:"url,omitempty"`
	Address string `json:"address,omitempty"`
	ChainID int    `json:"chainId,omitempty"`
}

// AssetInfo describes one payable asset for agent-to-agent discovery.
type AssetInfo struct {
	Symbol              string `json:"symbol"`
	Name                string `json:"name"`
	Decimals            int    `json:"decimals"`
	AssetTransferMethod string `json:"assetTransferMethod"` // "eip3009" or "permit2"
	TokenAddress        string `json:"tokenAddress"`
}

// Pricing summarises cost in human-readable form for discovery.
type Pricing struct {
	PricePerBlockUSD float64 `json:"price_per_block_usd"`
	BlockMinutes     float64 `json:"block_minutes"`
}

type AgentCard struct {
	Type             string      `json:"type"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Image            string      `json:"image,omitempty"`
	Endpoints        []Endpoint  `json:"endpoints"`
	SupportedTrust   []string    `json:"supportedTrust"`
	PaymentProtocol  string      `json:"payment_protocol"`
	PaymentNetwork   string      `json:"payment_network"`
	AcceptedAssets   []AssetInfo `json:"accepted_assets"`
	Runtimes         []string    `json:"runtimes"`
	Pricing          Pricing     `json:"pricing"`
}

func Build(cfg *config.Config) AgentCard {
	endpoints := []Endpoint{
		{Type: "wallet", Address: cfg.PayoutWallet, ChainID: int(cfg.CeloChainID)},
	}
	if cfg.AgentMCPPublicURL != "" {
		endpoints = append(endpoints, Endpoint{Type: "mcp", URL: cfg.AgentMCPPublicURL})
	}

	assets := []AssetInfo{
		{
			Symbol:              "USDC",
			Name:                "USD Coin",
			Decimals:            6,
			AssetTransferMethod: "eip3009",
			TokenAddress:        cfg.USDCTokenAddress,
		},
		{
			Symbol:              "USDT",
			Name:                "Tether USD",
			Decimals:            6,
			AssetTransferMethod: "eip3009",
			TokenAddress:        cfg.USDTTokenAddress,
		},
	}
	if cfg.WARSTokenAddress != "" {
		assets = append(assets, AssetInfo{
			Symbol:              "wARS",
			Name:                "Peso Argentino",
			Decimals:            18,
			AssetTransferMethod: "permit2",
			TokenAddress:        cfg.WARSTokenAddress,
		})
	}
	if cfg.WBRLTokenAddress != "" {
		assets = append(assets, AssetInfo{
			Symbol:              "wBRL",
			Name:                "Real Brasileiro",
			Decimals:            18,
			AssetTransferMethod: "permit2",
			TokenAddress:        cfg.WBRLTokenAddress,
		})
	}
	if cfg.WCOPTokenAddress != "" {
		assets = append(assets, AssetInfo{
			Symbol:              "wCOP",
			Name:                "Peso Colombiano",
			Decimals:            18,
			AssetTransferMethod: "permit2",
			TokenAddress:        cfg.WCOPTokenAddress,
		})
	}

	// Runtimes: node-build is always present; python-build and rust-build are
	// added when their adapter URLs are configured.
	runtimes := []string{"node-build"}
	if cfg.SandboxAdapterURLPython != "" {
		runtimes = append(runtimes, "python-build")
	}
	if cfg.SandboxAdapterURLRust != "" {
		runtimes = append(runtimes, "rust-build")
	}

	pricePerBlock, _ := strconv.ParseFloat(cfg.PricePerBlockUSD, 64)
	blockMinutes, _ := strconv.ParseFloat(cfg.BlockMinutes, 64)

	return AgentCard{
		Type:            "Agent",
		Name:            cfg.AgentName,
		Description:     cfg.AgentDescription,
		Image:           cfg.AgentImageURI,
		Endpoints:       endpoints,
		SupportedTrust:  []string{"reputation"},
		PaymentProtocol: "x402",
		PaymentNetwork:  cfg.CeloChainCAIP2,
		AcceptedAssets:  assets,
		Runtimes:        runtimes,
		Pricing: Pricing{
			PricePerBlockUSD: pricePerBlock,
			BlockMinutes:     blockMinutes,
		},
	}
}

func Handler(cfg *config.Config) http.HandlerFunc {
	card := Build(cfg)
	body, err := json.MarshalIndent(card, "", "  ")
	return func(w http.ResponseWriter, r *http.Request) {
		if err != nil {
			http.Error(w, "agent card build failed", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}
