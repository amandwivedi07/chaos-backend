package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ErrNotConfigured is returned when generative features are switched off.
var ErrNotConfigured = errors.New("ai is not configured")

// Config is what the Azure adapter needs. An empty Endpoint or APIKey means
// the caller should use Disabled instead.
type Config struct {
	Endpoint       string
	APIKey         string
	ChatDeployment string
	APIVersion     string
}

func (c Config) valid() bool {
	return c.Endpoint != "" && c.APIKey != "" &&
		c.ChatDeployment != "" && c.APIVersion != ""
}

type azureClient struct {
	cfg  Config
	http *http.Client
}

var _ Client = (*azureClient)(nil)

// NewAzure returns an Azure OpenAI adapter, or Disabled when the config is
// incomplete — a missing key must never take the whole server down.
func NewAzure(cfg Config) Client {
	if !cfg.valid() {
		return Disabled{}
	}
	cfg.Endpoint = strings.TrimSuffix(cfg.Endpoint, "/")
	return &azureClient{
		cfg:  cfg,
		http: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *azureClient) Enabled() bool { return true }

func (c *azureClient) url(path string) string {
	return fmt.Sprintf("%s/openai/deployments/%s/%s?api-version=%s",
		c.cfg.Endpoint, c.cfg.ChatDeployment, path, c.cfg.APIVersion)
}

// chat runs one completion and returns the assistant's raw content.
//
// NOTE: this model family rejects max_tokens and wants max_completion_tokens —
// swapping it back will 400.
func (c *azureClient) chat(ctx context.Context, system, user string, maxTokens int) (string, error) {
	// Azure rejects response_format=json_object unless the word "json" appears
	// somewhere in the messages. Every prompt below says it, but asserting it
	// here means a future prompt cannot quietly 400 in production.
	if !strings.Contains(strings.ToLower(system), "json") {
		return "", errors.New("system prompt must mention JSON to use json_object")
	}
	body := map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"max_completion_tokens": maxTokens,
		"response_format":       map[string]string{"type": "json_object"},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.url("chat/completions"), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		// Surface Azure's own message; it explains quota and filtering.
		var e struct {
			Error struct{ Message, Code string } `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Error.Message != "" {
			return "", fmt.Errorf("azure %d: %s", resp.StatusCode, e.Error.Message)
		}
		return "", fmt.Errorf("azure %d", resp.StatusCode)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("no completion returned")
	}
	return out.Choices[0].Message.Content, nil
}

// unfence strips the code fence a model adds despite being told not to, so the
// JSON underneath still parses.
func unfence(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)
	// Prose either side of the object: take the widest braced span.
	if open, close := strings.Index(s, "{"), strings.LastIndex(s, "}"); open >= 0 && close > open {
		return s[open : close+1]
	}
	return s
}
