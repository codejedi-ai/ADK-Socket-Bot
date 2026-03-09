package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/config"
	"github.com/codejedi-ai/adkgobot/internal/gateway"
	"github.com/codejedi-ai/adkgobot/internal/genai"
	"github.com/codejedi-ai/adkgobot/internal/media"
	"github.com/codejedi-ai/adkgobot/internal/proc"
	"github.com/codejedi-ai/adkgobot/internal/tui"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "adkbot",
		Short: "ADK-style websocket bot gateway and TUI",
	}

	root.AddCommand(newGatewayCmd())
	root.AddCommand(newTUICmd())
	root.AddCommand(newOnboardCmd())
	root.AddCommand(newManualCmd())
	root.AddCommand(newImageCmd())
	root.AddCommand(newVideoCmd())
	root.AddCommand(newTestCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newGatewayCmd() *cobra.Command {
	var host string
	var port int
	var model string
	var detach bool
	pidFile := config.PIDFile()
	defaultModel := config.ResolveGoogleModel()

	cmd := &cobra.Command{Use: "gateway", Short: "Gateway process controls"}

	start := &cobra.Command{
		Use:   "start",
		Short: "Start websocket gateway",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := ensureProviderSupported(); err != nil {
				return err
			}
			_ = proc.EnsureStopped(pidFile)

			if !detach {
				s := gateway.NewServer(host, port, model)
				return runWithSignals(s)
			}

			exe, err := os.Executable()
			if err != nil {
				return err
			}
			child := exec.Command(exe, "gateway", "serve", "--host", host, "--port", strconv.Itoa(port), "--model", model)
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			if err := child.Start(); err != nil {
				return err
			}
			if err := proc.WritePID(pidFile, child.Process.Pid); err != nil {
				return err
			}
			fmt.Printf("gateway started pid=%d ws://%s:%d/ws\n", child.Process.Pid, host, port)
			return nil
		},
	}
	start.Flags().StringVar(&host, "host", config.DefaultHost, "gateway host")
	start.Flags().IntVar(&port, "port", config.DefaultPort, "gateway port")
	start.Flags().StringVar(&model, "model", defaultModel, "Gemini model")
	start.Flags().BoolVar(&detach, "detach", true, "run in background")

	serve := &cobra.Command{
		Use:    "serve",
		Short:  "Run gateway server in foreground",
		Hidden: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := ensureProviderSupported(); err != nil {
				return err
			}
			s := gateway.NewServer(host, port, model)
			return runWithSignals(s)
		},
	}
	serve.Flags().StringVar(&host, "host", config.DefaultHost, "gateway host")
	serve.Flags().IntVar(&port, "port", config.DefaultPort, "gateway port")
	serve.Flags().StringVar(&model, "model", defaultModel, "Gemini model")

	stop := &cobra.Command{
		Use:   "stop",
		Short: "Stop running websocket gateway",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := proc.StopByPIDFile(pidFile); err != nil {
				return err
			}
			fmt.Println("gateway stopped")
			return nil
		},
	}

	restart := &cobra.Command{
		Use:   "restart",
		Short: "Restart gateway",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := ensureProviderSupported(); err != nil {
				return err
			}
			_ = proc.StopByPIDFile(pidFile)
			time.Sleep(300 * time.Millisecond)
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			child := exec.Command(exe, "gateway", "serve", "--host", host, "--port", strconv.Itoa(port), "--model", model)
			child.Stdout = os.Stdout
			child.Stderr = os.Stderr
			if err := child.Start(); err != nil {
				return err
			}
			if err := proc.WritePID(pidFile, child.Process.Pid); err != nil {
				return err
			}
			fmt.Printf("gateway restarted pid=%d ws://%s:%d/ws\n", child.Process.Pid, host, port)
			return nil
		},
	}
	restart.Flags().StringVar(&host, "host", config.DefaultHost, "gateway host")
	restart.Flags().IntVar(&port, "port", config.DefaultPort, "gateway port")
	restart.Flags().StringVar(&model, "model", defaultModel, "Gemini model")

	cmd.AddCommand(start, serve, stop, restart)
	return cmd
}

func ensureProviderSupported() error {
	cfg, err := config.LoadProviderConfig()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if cfg.Provider == "" || cfg.Provider == "google" {
		return nil
	}
	return fmt.Errorf("provider %q is configured, but runtime currently supports google only; run 'adkbot onboard' and choose google", cfg.Provider)
}

type shutdownable interface {
	Start() error
	Shutdown(ctx context.Context) error
}

func runWithSignals(s shutdownable) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		if err == nil || err.Error() == "http: Server closed" {
			return nil
		}
		return err
	case <-sigCh:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.Shutdown(ctx)
	}
}

func newTUICmd() *cobra.Command {
	var wsURL string
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Launch chat TUI against websocket gateway",
		RunE: func(_ *cobra.Command, _ []string) error {
			return tui.Run(wsURL)
		},
	}
	cmd.Flags().StringVar(&wsURL, "ws", config.DefaultGatewayWS, "gateway websocket URL")
	return cmd
}

func newManualCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "manual",
		Short: "Print command and protocol manual",
		RunE: func(_ *cobra.Command, _ []string) error {
			b, err := os.ReadFile("docs/MANUAL.md")
			if err != nil {
				return err
			}
			fmt.Println(string(b))
			return nil
		},
	}
}

func newOnboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Interactive provider and API key onboarding",
		RunE: func(_ *cobra.Command, _ []string) error {
			reader := bufio.NewReader(os.Stdin)
			fmt.Println("adkbot onboarding")
			existing, _ := config.LoadProviderConfig()

			googleStatus := providerStatusWhite("not configured")
			openAIStatus := providerStatusWhite("not configured")
			anthropicStatus := providerStatusWhite("not configured")
			cloudinaryStatus := providerStatusWhite("not configured")

			if strings.TrimSpace(existing.Google.APIKey) != "" {
				hctx, hcancel := context.WithTimeout(context.Background(), 8*time.Second)
				if err := genai.CheckAPIKeyHealth(hctx, strings.TrimSpace(existing.Google.APIKey)); err == nil {
					googleStatus = providerStatusGreen("configured and healthy")
				} else {
					googleStatus = providerStatusRed("configured but unhealthy")
				}
				hcancel()
			}
			if strings.TrimSpace(existing.OpenAI.APIKey) != "" {
				hctx, hcancel := context.WithTimeout(context.Background(), 8*time.Second)
				if err := checkOpenAIKeyHealth(hctx, strings.TrimSpace(existing.OpenAI.APIKey)); err == nil {
					openAIStatus = providerStatusGreen("configured and healthy")
				} else {
					openAIStatus = providerStatusRed("configured but unhealthy")
				}
				hcancel()
			}
			if strings.TrimSpace(existing.Anthropic.APIKey) != "" {
				hctx, hcancel := context.WithTimeout(context.Background(), 8*time.Second)
				if err := checkAnthropicKeyHealth(hctx, strings.TrimSpace(existing.Anthropic.APIKey)); err == nil {
					anthropicStatus = providerStatusGreen("configured and healthy")
				} else {
					anthropicStatus = providerStatusRed("configured but unhealthy")
				}
				hcancel()
			}
			if strings.TrimSpace(existing.Cloudinary.CloudName) != "" && strings.TrimSpace(existing.Cloudinary.APIKey) != "" && strings.TrimSpace(existing.Cloudinary.APISecret) != "" {
				cloudinaryStatus = providerStatusGreen("configured")
			}

			fmt.Println("Model category")
			fmt.Println("Select model provider: google | openai | anthropic | skip")
			fmt.Printf("  google    %s\n", googleStatus)
			fmt.Printf("  openai    %s\n", openAIStatus)
			fmt.Printf("  anthropic %s\n", anthropicStatus)
			fmt.Println("  skip      keep current provider config")
			fmt.Println()
			fmt.Println("Media category")
			fmt.Printf("  cloudinary %s\n", cloudinaryStatus)

			provider, err := prompt(reader, "Provider", "google")
			if err != nil {
				return err
			}
			provider = strings.ToLower(strings.TrimSpace(provider))
			if provider == "" {
				provider = "google"
			}
			switch provider {
			case "google", "openai", "anthropic", "skip":
			default:
				return errors.New("invalid provider; expected google, openai, anthropic, or skip")
			}

			cfg := existing
			if provider != "skip" {
				cfg.Provider = provider
			}

			switch provider {
			case "google":
				existingKey := strings.TrimSpace(existing.Google.APIKey)
				shouldConfigure := true
				if existingKey != "" {
					hctx, hcancel := context.WithTimeout(context.Background(), 8*time.Second)
					healthErr := genai.CheckAPIKeyHealth(hctx, existingKey)
					hcancel()
					if healthErr == nil {
						fmt.Println("Google is already configured and key is healthy.")
						configureNow, err := prompt(reader, "Configure Google now? (yes/no)", "no")
						if err != nil {
							return err
						}
						if !isYes(configureNow) {
							shouldConfigure = false
						}
					} else {
						fmt.Printf("%s\n", red("Google key is not healthy and needs reconfiguration."))
					}
				} else {
					fmt.Printf("%s\n", red("Google is not configured."))
				}

				if !shouldConfigure {
					break
				}

				defaultKey := existingKey
				key, err := prompt(reader, "Google API key", defaultKey)
				if err != nil {
					return err
				}
				if strings.TrimSpace(key) == "" {
					return errors.New("google API key cannot be empty")
				}
				modelDefault := config.ResolveGoogleModel()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				flashCurrent := "unavailable"
				proCurrent := "unavailable"
				if newest, derr := genai.DiscoverNewestFlashModel(ctx, strings.TrimSpace(key)); derr == nil && newest != "" {
					flashCurrent = newest
				}
				if newest, derr := genai.DiscoverNewestProModel(ctx, strings.TrimSpace(key)); derr == nil && newest != "" {
					proCurrent = newest
				}

				defaultMode := normalizeGoogleModelInput(modelDefault)
				if defaultMode == "" {
					defaultMode = normalizeGoogleModelInput(strings.TrimSpace(existing.Google.Model))
				}
				if defaultMode == "" {
					defaultMode = "auto-flash"
				}

				fmt.Printf("Google auto modes: auto-flash (%s), auto-pro (%s)\n", flashCurrent, proCurrent)
				model, err := prompt(reader, "Google model (auto-flash|auto-pro|custom|skip)", defaultMode)
				if err != nil {
					return err
				}
				model = strings.TrimSpace(model)
				if strings.EqualFold(model, "skip") {
					// keep existing model value untouched
					if strings.TrimSpace(cfg.Google.Model) == "" {
						cfg.Google.Model = defaultMode
					}
				} else {
					cfg.Google.Model = normalizeGoogleModelInput(model)
					if cfg.Google.Model == "" {
						cfg.Google.Model = defaultMode
					}
				}
				cfg.Google.APIKey = strings.TrimSpace(key)
			case "openai":
				fmt.Println("OpenAI onboarding scaffold is present, but runtime integration is not implemented yet.")
				model, err := prompt(reader, "OpenAI model", "gpt-4o-mini")
				if err != nil {
					return err
				}
				key, err := prompt(reader, "OpenAI API key", "")
				if err != nil {
					return err
				}
				cfg.OpenAI.Model = strings.TrimSpace(model)
				cfg.OpenAI.APIKey = strings.TrimSpace(key)
			case "anthropic":
				fmt.Println("Anthropic onboarding scaffold is present, but runtime integration is not implemented yet.")
				model, err := prompt(reader, "Anthropic model", "claude-3-5-sonnet-latest")
				if err != nil {
					return err
				}
				key, err := prompt(reader, "Anthropic API key", "")
				if err != nil {
					return err
				}
				cfg.Anthropic.Model = strings.TrimSpace(model)
				cfg.Anthropic.APIKey = strings.TrimSpace(key)
			case "skip":
				fmt.Println("Skipping provider configuration.")
			}

			hasCloudinary := strings.TrimSpace(existing.Cloudinary.CloudName) != "" || strings.TrimSpace(existing.Cloudinary.APIKey) != "" || strings.TrimSpace(existing.Cloudinary.APISecret) != ""
			cloudinaryDefault := "no"
			if hasCloudinary {
				cloudinaryDefault = "yes"
			}
			setupCloudinary, err := prompt(reader, "Set up Cloudinary? (yes/no)", cloudinaryDefault)
			if err != nil {
				return err
			}
			if isYes(setupCloudinary) {
				cloudName, err := prompt(reader, "Cloudinary cloud_name", strings.TrimSpace(existing.Cloudinary.CloudName))
				if err != nil {
					return err
				}
				apiKey, err := prompt(reader, "Cloudinary api_key", strings.TrimSpace(existing.Cloudinary.APIKey))
				if err != nil {
					return err
				}
				apiSecret, err := prompt(reader, "Cloudinary api_secret", strings.TrimSpace(existing.Cloudinary.APISecret))
				if err != nil {
					return err
				}
				cfg.Cloudinary.CloudName = strings.TrimSpace(cloudName)
				cfg.Cloudinary.APIKey = strings.TrimSpace(apiKey)
				cfg.Cloudinary.APISecret = strings.TrimSpace(apiSecret)
				cfg.Cloudinary.URL = ""
			} else {
				// Keep existing Cloudinary settings when user chooses no.
				cfg.Cloudinary = existing.Cloudinary
			}

			if err := config.SaveProviderConfig(cfg); err != nil {
				return err
			}
			fmt.Printf("saved onboarding config to %s\n", config.ConfigFile())
			if provider != "google" && provider != "skip" {
				fmt.Println("note: provider runtime integration currently available only for google")
			}
			return nil
		},
	}
}

func checkOpenAIKeyHealth(ctx context.Context, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.openai.com/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("openai health check failed with status %d", resp.StatusCode)
}

func checkAnthropicKeyHealth(ctx context.Context, apiKey string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.anthropic.com/v1/models", nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("anthropic health check failed with status %d", resp.StatusCode)
}

func providerStatusGreen(s string) string {
	return "\033[32m" + s + "\033[0m"
}

func providerStatusWhite(s string) string {
	return "\033[37m" + s + "\033[0m"
}

func providerStatusRed(s string) string {
	return "\033[31m" + s + "\033[0m"
}

func normalizeGoogleModelInput(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "auto", "auto-flash", "auto_flash", "auto flash", "auto - flash":
		return "auto-flash"
	case "auto-pro", "auto_pro", "auto pro", "auto - pro":
		return "auto-pro"
	default:
		return strings.TrimSpace(v)
	}
}

func prompt(reader *bufio.Reader, label, defaultVal string) (string, error) {
	if defaultVal != "" {
		fmt.Printf("%s [%s]: ", label, defaultVal)
	} else {
		fmt.Printf("%s: ", label)
	}
	text, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		if len(text) == 0 {
			return "", err
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return defaultVal, nil
	}
	return text, nil
}

func isYes(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "y" || v == "yes" || v == "true" || v == "1"
}

func red(s string) string {
	return "\033[31m" + s + "\033[0m"
}

func newImageCmd() *cobra.Command {
	var prompt string
	var model string
	var aspectRatio string
	var negativePrompt string
	var numImages int32
	var output string
	var upload bool
	var publicID string

	cmd := &cobra.Command{
		Use:   "image",
		Short: "Generate images using Gemini/Imagen and optionally upload to Cloudinary",
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(prompt) == "" {
				return errors.New("--prompt is required")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			opt := media.ImageOptions{
				Model:          model,
				AspectRatio:    aspectRatio,
				NegativePrompt: negativePrompt,
				NumberOfImages: numImages,
			}

			fmt.Printf("Generating image with prompt: %s\n", prompt)
			if opt.Model != "" {
				fmt.Printf("Model: %s\n", opt.Model)
			}

			result, err := media.GenerateImages(ctx, prompt, opt)
			if err != nil {
				return fmt.Errorf("image generation failed: %w", err)
			}

			images, ok := result["images"].([]map[string]any)
			if !ok || len(images) == 0 {
				return errors.New("no images generated")
			}

			for i, img := range images {
				mimeType, _ := img["mime_type"].(string)
				imageBase64, _ := img["image_base64"].(string)
				if strings.TrimSpace(imageBase64) == "" {
					return fmt.Errorf("generated image %d did not include image_base64", i+1)
				}
				data, err := base64.StdEncoding.DecodeString(imageBase64)
				if err != nil {
					return fmt.Errorf("failed to decode image %d: %w", i+1, err)
				}

				fmt.Printf("\n--- Image %d ---\n", i+1)
				fmt.Printf("MIME Type: %s\n", mimeType)

				if strings.TrimSpace(output) != "" {
					path := output
					if len(images) > 1 {
						path = indexedOutputPath(output, i+1)
					}
					if err := os.WriteFile(path, data, 0o644); err != nil {
						return fmt.Errorf("failed writing image to %s: %w", path, err)
					}
					fmt.Printf("Saved image: %s\n", path)
				}

				if upload {
					uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 30*time.Second)
					uploadPID := strings.TrimSpace(publicID)
					if uploadPID != "" && len(images) > 1 {
						uploadPID = fmt.Sprintf("%s_%d", uploadPID, i+1)
					}

					uploadResult, err := media.UploadBytes(uploadCtx, data, mimeType, uploadPID, "image")
					uploadCancel()
					if err != nil {
						return fmt.Errorf("upload failed: %w", err)
					}

					fmt.Printf("Uploaded to Cloudinary:\n")
					fmt.Printf("  Public ID: %s\n", uploadResult["public_id"])
					fmt.Printf("  URL: %s\n", uploadResult["secure_url"])
				} else {
					dataURL, _ := img["data_url"].(string)
					fmt.Printf("Data URL (first 100 chars): %s...\n", truncate(dataURL, 100))
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "Image generation prompt (required)")
	cmd.Flags().StringVarP(&model, "model", "m", "", "Image model (default: gemini-3.1-flash-image-preview)")
	cmd.Flags().StringVarP(&aspectRatio, "aspect", "a", "", "Aspect ratio (e.g., 16:9, 1:1, 9:16)")
	cmd.Flags().StringVarP(&negativePrompt, "negative", "n", "", "Negative prompt")
	cmd.Flags().Int32VarP(&numImages, "count", "c", 1, "Number of images to generate")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (when --count > 1, suffixes like _1 are added)")
	cmd.Flags().BoolVarP(&upload, "upload", "u", false, "Upload generated images to Cloudinary")
	cmd.Flags().StringVar(&publicID, "public-id", "", "Cloudinary public ID (optional)")

	return cmd
}

func newVideoCmd() *cobra.Command {
	var prompt string
	var model string
	var aspectRatio string
	var resolution string
	var negativePrompt string
	var durationSec int32
	var numVideos int32
	var upload bool
	var publicID string
	var wait bool

	cmd := &cobra.Command{
		Use:   "video",
		Short: "Generate videos using Gemini/Veo and optionally upload to Cloudinary",
		RunE: func(_ *cobra.Command, _ []string) error {
			if strings.TrimSpace(prompt) == "" {
				return errors.New("--prompt is required")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
			defer cancel()

			opt := media.VideoOptions{
				Model:           model,
				AspectRatio:     aspectRatio,
				Resolution:      resolution,
				NegativePrompt:  negativePrompt,
				DurationSeconds: durationSec,
				NumberOfVideos:  numVideos,
				Wait:            wait,
			}

			fmt.Printf("Generating video with prompt: %s\n", prompt)
			if opt.Model != "" {
				fmt.Printf("Model: %s\n", opt.Model)
			}

			result, err := media.GenerateVideos(ctx, prompt, opt)
			if err != nil {
				return fmt.Errorf("video generation failed: %w", err)
			}

			fmt.Printf("\nOperation: %s\n", result["operation_name"])
			fmt.Printf("Status: %v\n", result["done"])

			if note, ok := result["note"].(string); ok && note != "" {
				fmt.Printf("Note: %s\n", note)
			}

			videos, ok := result["videos"].([]map[string]any)
			if ok && len(videos) > 0 {
				for i, vid := range videos {
					fmt.Printf("\n--- Video %d ---\n", i+1)
					if uri, ok := vid["uri"].(string); ok {
						fmt.Printf("URI: %s\n", uri)
					}
					if mimeType, ok := vid["mime_type"].(string); ok {
						fmt.Printf("MIME Type: %s\n", mimeType)
					}

					if upload {
						if videoBase64, ok := vid["video_base64"].(string); ok && videoBase64 != "" {
							data, err := base64.StdEncoding.DecodeString(videoBase64)
							if err != nil {
								return fmt.Errorf("failed to decode video: %w", err)
							}

							uploadCtx, uploadCancel := context.WithTimeout(context.Background(), 120*time.Second)
							defer uploadCancel()

							mimeType := "video/mp4"
							if mt, ok := vid["mime_type"].(string); ok {
								mimeType = mt
							}

							uploadPID := publicID
							if uploadPID == "" {
								uploadPID = fmt.Sprintf("adkbot_video_%d", time.Now().UnixNano())
							}

							uploadResult, err := media.UploadBytes(uploadCtx, data, mimeType, uploadPID, "video")
							if err != nil {
								return fmt.Errorf("upload failed: %w", err)
							}

							fmt.Printf("Uploaded to Cloudinary:\n")
							fmt.Printf("  Public ID: %s\n", uploadResult["public_id"])
							fmt.Printf("  URL: %s\n", uploadResult["secure_url"])
						} else {
							fmt.Println("Video data not available for upload (requires wait=true)")
						}
					}
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "Video generation prompt (required)")
	cmd.Flags().StringVarP(&model, "model", "m", "", "Video model (default: veo-3.1-generate-preview)")
	cmd.Flags().StringVarP(&aspectRatio, "aspect", "a", "", "Aspect ratio (e.g., 16:9, 1:1, 9:16)")
	cmd.Flags().StringVarP(&resolution, "resolution", "r", "", "Resolution (e.g., 720p, 1080p)")
	cmd.Flags().StringVarP(&negativePrompt, "negative", "n", "", "Negative prompt")
	cmd.Flags().Int32VarP(&durationSec, "duration", "d", 5, "Duration in seconds")
	cmd.Flags().Int32VarP(&numVideos, "count", "c", 1, "Number of videos to generate")
	cmd.Flags().BoolVarP(&upload, "upload", "u", false, "Upload generated videos to Cloudinary")
	cmd.Flags().StringVar(&publicID, "public-id", "", "Cloudinary public ID (optional)")
	cmd.Flags().BoolVarP(&wait, "wait", "w", true, "Wait for video generation to complete")

	return cmd
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test Cloudinary upload and media functionality",
		RunE: func(_ *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			fmt.Println("=== Test 1: Upload from remote URL ===")
			result, err := media.UploadRemote(ctx, "https://picsum.photos/800/600", "test_remote_image", "image")
			if err != nil {
				return fmt.Errorf("upload failed: %w", err)
			}
			fmt.Printf("✓ Upload successful!\n")
			fmt.Printf("  Public ID: %v\n", result["public_id"])
			fmt.Printf("  URL: %v\n", result["secure_url"])
			fmt.Printf("  Resource Type: %v\n", result["resource_type"])

			fmt.Println("\n=== Test 2: Upload from bytes ===")
			ctx2, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel2()

			pngData := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0xf0, 0x00, 0x00, 0x00, 0x03, 0x00, 0x01, 0x00, 0x05, 0xfe, 0xd4, 0xef, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82}

			result2, err := media.UploadBytes(ctx2, pngData, "image/png", "test_pixel_image", "image")
			if err != nil {
				return fmt.Errorf("upload failed: %w", err)
			}
			fmt.Printf("✓ Upload successful!\n")
			fmt.Printf("  Public ID: %v\n", result2["public_id"])
			fmt.Printf("  URL: %v\n", result2["secure_url"])

			fmt.Println("\n=== All tests passed! ===")
			return nil
		},
	}
	return cmd
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

func indexedOutputPath(path string, index int) string {
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	if ext == "" {
		return fmt.Sprintf("%s_%d", base, index)
	}
	return fmt.Sprintf("%s_%d%s", base, index, ext)
}
