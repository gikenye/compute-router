// Command server runs the MCP + x402 resource server for the compute
// sandbox product. See /docs/SPEC-100.md before changing anything here,
// and /docs/ADR-001-language-choice.md for why this is Go and not
// TypeScript.
package main

import (
	"log"
	"net/http"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/YOUR_ORG/compute-router/services/core/internal/config"
	appmcp "github.com/YOUR_ORG/compute-router/services/core/internal/mcp"
	"github.com/YOUR_ORG/compute-router/services/core/internal/payment"
	"github.com/YOUR_ORG/compute-router/services/core/internal/sandboxclient"
	"github.com/YOUR_ORG/compute-router/services/core/internal/session"
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
		Sandbox:  sandboxclient.New(cfg.SandboxAdapterURL, cfg.SandboxAdapterSecret),
	}

	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "compute-router",
		Version: "0.1.0",
	}, nil)

	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "provision_env",
		Description: "Provision a fresh sandboxed shell with preinstalled build tools. Costs USDC on Celo, billed at provision time for a fixed block (see SPEC-100 §4.4 for the metered-billing stretch goal).",
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

	// Streamable HTTP transport — confirmed API from
	// github.com/modelcontextprotocol/go-sdk. This is what makes the
	// service network-callable (and therefore deployable to the cloud)
	// rather than a stdio-only local subprocess.
	handler := gomcp.NewStreamableHTTPHandler(func(r *http.Request) *gomcp.Server {
		return server
	}, nil)

	log.Printf("compute-router listening on :%s", cfg.Port)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}
