package genai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/config"
)

const (
	DefaultImageGenModel = "gemini-2.0-flash-preview-image-generation"
)

func GenerateImage(ctx context.Context, prompt, model string) (map[string]any, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	apiKey, err := config.ResolveGoogleAPIKey()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(model) == "" {
		if v := os.Getenv("ADKBOT_IMAGE_MODEL"); v != "" {
			model = v
		} else {
			model = DefaultImageGenModel
		}
	}

	payload := map[string]any{
		"contents": []map[string]any{{
			"role":  "user",
			"parts": []map[string]string{{"text": prompt}},
		}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"TEXT", "IMAGE"},
		},
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s", model, apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini image generation error: %s", string(body))
	}

	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text       string `json:"text"`
					InlineData struct {
						MimeType string `json:"mimeType"`
						Data     string `json:"data"`
					} `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	if len(out.Candidates) == 0 {
		return nil, errors.New("no candidates returned from Gemini image generation")
	}

	var textParts []string
	mime := ""
	imageB64 := ""
	for _, p := range out.Candidates[0].Content.Parts {
		if strings.TrimSpace(p.Text) != "" {
			textParts = append(textParts, p.Text)
		}
		if p.InlineData.Data != "" {
			mime = p.InlineData.MimeType
			imageB64 = p.InlineData.Data
		}
	}
	if imageB64 == "" {
		return nil, errors.New("Gemini response did not include inline image data")
	}

	return map[string]any{
		"model":        model,
		"text":         strings.Join(textParts, "\n"),
		"mime_type":    mime,
		"image_base64": imageB64,
		"data_url":     "data:" + mime + ";base64," + imageB64,
	}, nil
}

func StartVideoGenerationJob(ctx context.Context, prompt, model string) (map[string]any, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	endpoint := os.Getenv("GOOGLE_VIDEO_GEN_ENDPOINT")
	token := os.Getenv("GOOGLE_OAUTH_ACCESS_TOKEN")
	if endpoint == "" || token == "" {
		return nil, errors.New("video generation requires GOOGLE_VIDEO_GEN_ENDPOINT and GOOGLE_OAUTH_ACCESS_TOKEN (Vertex/Gemini video endpoint)")
	}
	if model == "" {
		model = "veo-2.0-generate-001"
	}

	payload := map[string]any{
		"prompt": prompt,
		"model":  model,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	client := http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini video job start failed: %s", string(body))
	}
	return map[string]any{
		"status":   resp.StatusCode,
		"response": string(body),
		"note":     "Use returned operation/job ID with your configured endpoint to poll completion.",
	}, nil
}
