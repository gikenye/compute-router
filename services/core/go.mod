module github.com/YOUR_ORG/compute-router/services/core

go 1.25.0

// NOT populated here — this tool environment has no network access, so
// no go.sum could be generated or verified. Run `make deps` (see root
// Makefile) locally before anything else. That runs:
//   go get github.com/modelcontextprotocol/go-sdk/mcp@latest
//   go mod tidy
// Confirmed real import path (checked against pkg.go.dev and the SDK's
// own GitHub repo directly): github.com/modelcontextprotocol/go-sdk/mcp

require github.com/modelcontextprotocol/go-sdk v1.7.0

require (
	github.com/google/jsonschema-go v0.4.3 // indirect
	github.com/segmentio/asm v1.1.3 // indirect
	github.com/segmentio/encoding v0.5.4 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/oauth2 v0.35.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/time v0.15.0 // indirect
)
