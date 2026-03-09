package genai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/config"
)

type Client struct {
	apiKey string
	keyErr error
	model  string
	auto   string
	http   http.Client
	mu     sync.RWMutex
}

func NewClient(model string) *Client {
	if model == "" {
		model = config.DefaultModel
	}
	apiKey, keyErr := config.ResolveGoogleAPIKey()
	auto := normalizeAutoMode(model)
	resolved := model
	if keyErr == nil && auto != "" {
		if m, err := discoverModelByAutoMode(context.Background(), apiKey, auto); err == nil && m != "" {
			resolved = m
		}
	}
	return &Client{
		apiKey: apiKey,
		keyErr: keyErr,
		model:  resolved,
		auto:   auto,
		http:   http.Client{Timeout: 120 * time.Second},
	}
}

func (c *Client) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if c.apiKey == "" {
		if c.keyErr != nil {
			return "", c.keyErr
		}
		return "", errors.New("google API key is not set; run 'adkbot onboard'")
	}

	payload := map[string]any{
		"system_instruction": map[string]any{
			"parts": []map[string]string{{"text": systemPrompt}},
		},
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]string{{"text": userPrompt}},
			},
		},
	}

	model := c.currentModel()
	text, statusCode, body, err := c.generateWithModel(ctx, model, payload)
	if err == nil {
		return text, nil
	}

	if !isModelUnavailable(statusCode, body, err) {
		return "", err
	}

	autoMode := c.autoMode()
	if autoMode == "" {
		autoMode = "auto-flash"
	}
	fallback, derr := discoverModelByAutoMode(ctx, c.apiKey, autoMode)
	if derr != nil {
		return "", fmt.Errorf("configured model %q is unavailable and model discovery failed: %w", model, derr)
	}
	if fallback == "" || fallback == model {
		return "", err
	}

	c.setCurrentModel(fallback)
	text, _, _, err = c.generateWithModel(ctx, fallback, payload)
	if err != nil {
		return "", err
	}
	return text, nil
}

func (c *Client) currentModel() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.model
}

func (c *Client) setCurrentModel(m string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.model = m
}

func (c *Client) autoMode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.auto
}

func (c *Client) generateWithModel(ctx context.Context, model string, payload map[string]any) (string, int, string, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return "", 0, "", err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, "", err
	}
	if resp.StatusCode >= 300 {
		return "", resp.StatusCode, string(body), fmt.Errorf("gemini API error (%d): %s", resp.StatusCode, string(body))
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", resp.StatusCode, string(body), err
	}
	if len(out.Candidates) == 0 || len(out.Candidates[0].Content.Parts) == 0 {
		return "", resp.StatusCode, string(body), errors.New("no response candidates from Gemini")
	}
	return out.Candidates[0].Content.Parts[0].Text, resp.StatusCode, string(body), nil
}

func supportsMethod(methods []string, method string) bool {
	for _, m := range methods {
		if m == method {
			return true
		}
	}
	return false
}

func isModelUnavailable(statusCode int, body string, err error) bool {
	if statusCode == http.StatusNotFound {
		return true
	}
	lb := strings.ToLower(body)
	if strings.Contains(lb, "not found") || strings.Contains(lb, "not supported") || strings.Contains(lb, "decommission") || strings.Contains(lb, "deprecated") {
		return true
	}
	le := strings.ToLower(err.Error())
	if strings.Contains(le, "not found") || strings.Contains(le, "decommission") {
		return true
	}
	return false
}

func normalizeAutoMode(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "auto", "auto-flash", "auto_flash", "auto flash":
		return "auto-flash"
	case "auto-pro", "auto_pro", "auto pro":
		return "auto-pro"
	default:
		return ""
	}
}

func discoverModelByAutoMode(ctx context.Context, apiKey, mode string) (string, error) {
	switch normalizeAutoMode(mode) {
	case "auto-pro":
		return DiscoverNewestProModel(ctx, apiKey)
	case "auto-flash":
		fallthrough
	default:
		return DiscoverNewestFlashModel(ctx, apiKey)
	}
}
