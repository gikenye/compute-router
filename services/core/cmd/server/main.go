// Command server runs the MCP and x402 resource server.
package main

import (
	"encoding/json"
	"log"
	"net/http"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/gikenye/compute-router/services/core/internal/agentcard"
	"github.com/gikenye/compute-router/services/core/internal/config"
	appmcp "github.com/gikenye/compute-router/services/core/internal/mcp"
	"github.com/gikenye/compute-router/services/core/internal/payment"
	"github.com/gikenye/compute-router/services/core/internal/refunds"
	"github.com/gikenye/compute-router/services/core/internal/safety"
	"github.com/gikenye/compute-router/services/core/internal/sandboxclient"
	"github.com/gikenye/compute-router/services/core/internal/session"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	deps := &appmcp.Deps{
		Cfg:      cfg,
		Sessions: session.NewStore(),
		Pay:      payment.New(cfg.FacilitatorURL, cfg.FacilitatorAPIKey),
		Refunds:  refunds.New(cfg.RefundLedgerPath, mustRefundSender(cfg), cfg.AttributionTag),
		Sandbox:  sandboxclient.New(cfg.SandboxAdapterURL, cfg.SandboxAdapterSecret),
		Safety:   safety.New(cfg),
	}

	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "compute-router",
		Version: "0.1.0",
	}, nil)

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "provision_env",
		Description: "Provision a fresh sandboxed shell with preinstalled build tools. Costs USDC or USDT on Celo, billed at provision time for a fixed block.",
	}, deps.ProvisionEnv)

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "exec",
		Description: "Run a shell command in an active session.",
	}, deps.Exec)

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "extend_ceiling",
		Description: "Extend a session's lease by another priced block.",
	}, deps.ExtendCeiling)

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "release",
		Description: "End a session immediately.",
	}, deps.Release)

	mcpHandler := gomcp.NewStreamableHTTPHandler(func(r *http.Request) *gomcp.Server {
		return server
	}, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-card.json", agentcard.Handler(cfg))
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.Header.Get("Mcp-Session-Id") == "" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":        "Compute Router",
				"description": "MCP server for paid, short-lived compute sandboxes.",
				"protocol":    "MCP Streamable HTTP",
				"endpoint":    "/mcp",
				"agent_card":  "/.well-known/agent-card.json",
				"usage":       "POST JSON-RPC initialize to start an MCP session.",
			})
			return
		}
		mcpHandler.ServeHTTP(w, r)
	})
	mux.Handle("/", http.FileServer(http.Dir("./web")))

	log.Printf("compute-router listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatal(err)
	}

}

func mustRefundSender(cfg *config.Config) *refunds.Sender {
	sender, err := refunds.NewSender(cfg.CeloRPCURL, cfg.RefundOperatorPrivateKey, cfg.PayoutWallet, cfg.CeloChainID)
	if err != nil {
		log.Fatalf("refund configuration error: %v", err)
	}
	return sender
}
