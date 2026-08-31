// Package sandboxclient is the ONLY place in this service that knows
// services/sandbox-adapter exists. Plain HTTP, shared-secret authed.
// See /docs/ADR-001-language-choice.md for why this boundary exists.
package sandboxclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Client struct {
	BaseURL string
	Secret  string
	HTTP    *http.Client
}

func New(baseURL, secret string) *Client {
	return &Client{BaseURL: baseURL, Secret: secret, HTTP: &http.Client{}}
}

func (c *Client) post(path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Adapter-Secret", c.Secret)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("adapter %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}
	if out != nil {
		return json.Unmarshal(respBody, out)
	}
	return nil
}

// Boot provisions a fresh sandbox for the given session ID. NOTE
// (SPEC-100 update): template selection at runtime is NOT supported by
// Cloudflare's container model — the image is fixed at Worker deploy
// time. This call passes template through for forward-compatibility but
// the MVP adapter ignores it and always boots the single deployed image.
func (c *Client) Boot(sessionID, template string) error {
	return c.post("/boot", map[string]string{
		"session_id": sessionID,
		"template":   template,
	}, nil)
}

type ExecResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
}

func (c *Client) Exec(sessionID, command string) (*ExecResult, error) {
	var result ExecResult
	err := c.post("/exec", map[string]string{
		"session_id": sessionID,
		"command":    command,
	}, &result)
	return &result, err
}

func (c *Client) Destroy(sessionID string) error {
	return c.post("/destroy", map[string]string{
		"session_id": sessionID,
	}, nil)
}
