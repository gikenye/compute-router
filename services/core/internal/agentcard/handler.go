// Package agentcard serves the ERC-8004 agent registration file.
package agentcard

import (
	"encoding/json"
	"net/http"

	"github.com/gikenye/compute-router/services/core/internal/config"
)

type Endpoint struct {
	Type    string `json:"type"`
	URL     string `json:"url,omitempty"`
	Address string `json:"address,omitempty"`
	ChainID int    `json:"chainId,omitempty"`
}

type AgentCard struct {
	Type           string     `json:"type"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Image          string     `json:"image,omitempty"`
	Endpoints      []Endpoint `json:"endpoints"`
	SupportedTrust []string   `json:"supportedTrust"`
}

func Build(cfg *config.Config) AgentCard {
	endpoints := []Endpoint{
		{Type: "wallet", Address: cfg.PayoutWallet, ChainID: int(cfg.CeloChainID)},
	}
	if cfg.AgentMCPPublicURL != "" {
		endpoints = append(endpoints, Endpoint{Type: "mcp", URL: cfg.AgentMCPPublicURL})
	}

	return AgentCard{
		Type:           "Agent",
		Name:           cfg.AgentName,
		Description:    cfg.AgentDescription,
		Image:          cfg.AgentImageURI,
		Endpoints:      endpoints,
		SupportedTrust: []string{"reputation"},
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
