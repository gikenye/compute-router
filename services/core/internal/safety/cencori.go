// Package safety implements an optional pre-exec command check.
package safety

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gikenye/compute-router/services/core/internal/config"
)

type Checker struct {
	cfg    *config.Config
	client *http.Client
}

func New(cfg *config.Config) *Checker {
	return &Checker{
		cfg:    cfg,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

type checkResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

func (c *Checker) Check(command string) (allowed bool, reason string, err error) {
	if !c.cfg.SafetyCheckEnabled {
		return true, "", nil
	}

	reqBody, _ := json.Marshal(map[string]any{
		"model": "gateway-default",
		"messages": []map[string]string{
			{"role": "system", "content": "You are a command-safety classifier for a shared compute sandbox. Reply with exactly ALLOW or DENY, optionally followed by a one-sentence reason. Deny commands that could exfiltrate secrets, attack other tenants, or abuse the host — not merely unusual commands."},
			{"role": "user", "content": command},
		},
	})

	req, reqErr := http.NewRequest("POST", c.cfg.CencoriAPIBaseURL+"/v1/chat/completions", bytes.NewReader(reqBody))
	if reqErr != nil {
		return c.cfg.SafetyCheckFailOpen, "", reqErr
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.CencoriAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, doErr := c.client.Do(req)
	if doErr != nil {
		return c.cfg.SafetyCheckFailOpen, "", fmt.Errorf("cencori unreachable, failing %s: %w",
			map[bool]string{true: "open", false: "closed"}[c.cfg.SafetyCheckFailOpen], doErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.cfg.SafetyCheckFailOpen, "", fmt.Errorf("cencori returned %d", resp.StatusCode)
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil || len(parsed.Choices) == 0 {
		return c.cfg.SafetyCheckFailOpen, "", fmt.Errorf("unparseable cencori response")
	}

	content := parsed.Choices[0].Message.Content
	result := checkResult{Allowed: len(content) >= 5 && content[:5] == "ALLOW"}
	if !result.Allowed {
		result.Reason = content
	}
	return result.Allowed, result.Reason, nil
}
