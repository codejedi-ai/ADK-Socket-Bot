package media

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/config"

	"google.golang.org/genai"
)

const (
	DefaultImageModel = "gemini-3.1-flash-image-preview"
	DefaultVideoModel = "veo-3.1-generate-preview"
)

type ImageOptions struct {
	Model          string
	Backend        string
	AspectRatio    string
	NegativePrompt string
	NumberOfImages int32
}

type VideoOptions struct {
	Model           string
	Backend         string
	AspectRatio     string
	Resolution      string
	DurationSeconds int32
	NumberOfVideos  int32
	NegativePrompt  string
	Wait            bool
	PollIntervalSec int
	TimeoutSec      int
}

func newGenAIClient(ctx context.Context, backend string) (*genai.Client, error) {
	b := strings.ToLower(strings.TrimSpace(backend))
	switch b {
	case "", "gemini", "google", "googleai", "geminiapi":
		apiKey, err := config.ResolveGoogleAPIKey()
		if err != nil {
			return nil, err
		}
		return genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey, Backend: genai.BackendGeminiAPI})
	case "vertex", "vertexai":
		project := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT"))
		location := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_LOCATION"))
		if location == "" {
			location = strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_REGION"))
		}
		if project == "" || location == "" {
			return nil, errors.New("vertex backend requires GOOGLE_CLOUD_PROJECT and GOOGLE_CLOUD_LOCATION/GOOGLE_CLOUD_REGION")
		}
		cc := &genai.ClientConfig{Backend: genai.BackendVertexAI, Project: project, Location: location}
		return genai.NewClient(ctx, cc)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", backend)
	}
}

func GenerateImages(ctx context.Context, prompt string, opt ImageOptions) (map[string]any, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	if strings.TrimSpace(opt.Model) == "" {
		if v := strings.TrimSpace(os.Getenv("ADKBOT_IMAGE_MODEL")); v != "" {
			opt.Model = v
		} else {
			opt.Model = DefaultImageModel
		}
	}
	client, err := newGenAIClient(ctx, opt.Backend)
	if err != nil {
		return nil, err
	}

	cfg := &genai.GenerateContentConfig{}
	if strings.TrimSpace(opt.AspectRatio) != "" {
		cfg.ResponseModalities = []string{"TEXT", "IMAGE"}
	}
	if opt.NumberOfImages > 0 {
		cfg.CandidateCount = int32(opt.NumberOfImages)
	}

	resp, err := client.Models.GenerateContent(ctx, opt.Model, []*genai.Content{
		{Role: "user", Parts: []*genai.Part{{Text: prompt}}},
	}, cfg)
	if err != nil {
		return nil, err
	}

	items := []map[string]any{}
	for _, candidate := range resp.Candidates {
		if candidate.Content == nil || candidate.Content.Parts == nil {
			continue
		}
		for _, part := range candidate.Content.Parts {
			if part.InlineData != nil && len(part.InlineData.Data) > 0 {
				mime := part.InlineData.MIMEType
				if mime == "" {
					mime = "image/png"
				}
				data := part.InlineData.Data
				b64 := base64.StdEncoding.EncodeToString(data)

				// Save locally
				localPath, err := saveMediaLocally(data, mime, "image")
				if err != nil {
					fmt.Printf("Warning: failed to save image locally: %v\n", err)
				}

				// Log to CSV
				LogMediaMetadata(prompt, localPath, "", "image")

				items = append(items, map[string]any{
					"mime_type":    mime,
					"image_base64": b64,
					"data_url":     "data:" + mime + ";base64," + b64,
					"local_path":   localPath,
				})
			}
		}
	}

	if len(items) == 0 {
		return nil, errors.New("no images returned by model")
	}
	return map[string]any{"model": opt.Model, "images": items}, nil
}

func GenerateVideos(ctx context.Context, prompt string, opt VideoOptions) (map[string]any, error) {
	if strings.TrimSpace(prompt) == "" {
		return nil, errors.New("prompt is required")
	}
	if strings.TrimSpace(opt.Model) == "" {
		if v := strings.TrimSpace(os.Getenv("ADKBOT_VIDEO_MODEL")); v != "" {
			opt.Model = v
		} else {
			opt.Model = DefaultVideoModel
		}
	}
	client, err := newGenAIClient(ctx, opt.Backend)
	if err != nil {
		return nil, err
	}

	cfg := &genai.GenerateVideosConfig{}
	if strings.TrimSpace(opt.AspectRatio) != "" {
		cfg.AspectRatio = opt.AspectRatio
	}
	if strings.TrimSpace(opt.Resolution) != "" {
		cfg.Resolution = opt.Resolution
	}
	if strings.TrimSpace(opt.NegativePrompt) != "" {
		cfg.NegativePrompt = opt.NegativePrompt
	}
	if opt.DurationSeconds > 0 {
		v := opt.DurationSeconds
		cfg.DurationSeconds = &v
	}
	if opt.NumberOfVideos > 0 {
		cfg.NumberOfVideos = opt.NumberOfVideos
	}

	op, err := client.Models.GenerateVideos(ctx, opt.Model, prompt, nil, cfg)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"model":          opt.Model,
		"operation_name": op.Name,
		"done":           op.Done,
	}
	if !opt.Wait {
		result["note"] = "Use pollin tool with operation_name to poll or set wait=true."
		return result, nil
	}

	pollSec := opt.PollIntervalSec
	if pollSec <= 0 {
		pollSec = 5
	}
	timeoutSec := opt.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	deadline := time.Now().Add(time.Duration(timeoutSec) * time.Second)
	for !op.Done && time.Now().Before(deadline) {
		time.Sleep(time.Duration(pollSec) * time.Second)
		op, err = client.Operations.GetVideosOperation(ctx, &genai.GenerateVideosOperation{Name: op.Name}, nil)
		if err != nil {
			return nil, err
		}
	}

	result["done"] = op.Done
	if !op.Done {
		result["note"] = "timeout reached before completion"
		return result, nil
	}
	if op.Error != nil {
		result["error"] = op.Error
		return result, nil
	}

	videos := []map[string]any{}
	if op.Response != nil {
		for _, gv := range op.Response.GeneratedVideos {
			if gv == nil || gv.Video == nil {
				continue
			}
			item := map[string]any{
				"uri":       gv.Video.URI,
				"mime_type": gv.Video.MIMEType,
			}

			var data []byte
			if len(gv.Video.VideoBytes) > 0 {
				data = gv.Video.VideoBytes
				item["video_base64"] = base64.StdEncoding.EncodeToString(data)
			}

			// Save locally
			localPath, err := saveMediaLocally(data, gv.Video.MIMEType, "video")
			if err != nil {
				fmt.Printf("Warning: failed to save video locally: %v\n", err)
			}
			item["local_path"] = localPath

			// Log to CSV
			LogMediaMetadata(prompt, localPath, gv.Video.URI, "video")

			videos = append(videos, item)
		}
	}
	result["videos"] = videos
	return result, nil
}

func saveMediaLocally(data []byte, mimeType string, kind string) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	dir := filepath.Join(config.RuntimeDir(), ".media")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	ext := "bin"
	switch {
	case strings.Contains(mimeType, "png"):
		ext = "png"
	case strings.Contains(mimeType, "jpg") || strings.Contains(mimeType, "jpeg"):
		ext = "jpg"
	case strings.Contains(mimeType, "mp4"):
		ext = "mp4"
	case kind == "image":
		ext = "png"
	case kind == "video":
		ext = "mp4"
	}

	filename := fmt.Sprintf("%s_%d.%s", kind, time.Now().UnixNano(), ext)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func LogMediaMetadata(prompt, localPath, remoteURL, kind string) {
	dir := filepath.Join(config.RuntimeDir(), ".media")
	_ = os.MkdirAll(dir, 0755)

	csvPath := filepath.Join(dir, "metadata.csv")
	f, err := os.OpenFile(csvPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	// Simple CSV escaping: replace quotes with double quotes and wrap in quotes
	escape := func(s string) string {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}

	line := fmt.Sprintf("%s,%s,%s,%s,%s\n",
		escape(time.Now().Format(time.RFC3339)),
		escape(kind),
		escape(prompt),
		escape(localPath),
		escape(remoteURL),
	)

	_, _ = f.WriteString(line)
}

func PollVideoOperation(ctx context.Context, operationName, backend string) (map[string]any, error) {
	if strings.TrimSpace(operationName) == "" {
		return nil, errors.New("operation_name is required")
	}
	client, err := newGenAIClient(ctx, backend)
	if err != nil {
		return nil, err
	}
	op, err := client.Operations.GetVideosOperation(ctx, &genai.GenerateVideosOperation{Name: operationName}, nil)
	if err != nil {
		return nil, err
	}
	result := map[string]any{"operation_name": op.Name, "done": op.Done}
	if op.Error != nil {
		result["error"] = op.Error
	}
	if op.Response != nil {
		videos := []map[string]any{}
		for _, gv := range op.Response.GeneratedVideos {
			if gv == nil || gv.Video == nil {
				continue
			}
			item := map[string]any{"uri": gv.Video.URI, "mime_type": gv.Video.MIMEType}
			if len(gv.Video.VideoBytes) > 0 {
				item["video_base64"] = base64.StdEncoding.EncodeToString(gv.Video.VideoBytes)
			}
			videos = append(videos, item)
		}
		result["videos"] = videos
	}
	return result, nil
}
