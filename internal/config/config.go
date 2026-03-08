package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultHost      = "127.0.0.1"
	DefaultPort      = 8080
	DefaultModel     = "auto-flash"
	DefaultGatewayWS = "ws://127.0.0.1:8080/ws"
)

func RuntimeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".adkbot"
	}
	return filepath.Join(home, ".adkbot")
}

func PIDFile() string {
	return filepath.Join(RuntimeDir(), "gateway.pid")
}

func ConfigFile() string {
	return filepath.Join(RuntimeDir(), "config.json")
}

type ProviderConfig struct {
	Provider   string `json:"provider"`
	Cloudinary struct {
		URL       string `json:"url,omitempty"`
		CloudName string `json:"cloudName,omitempty"`
		APIKey    string `json:"apiKey,omitempty"`
		APISecret string `json:"apiSecret,omitempty"`
	} `json:"cloudinary,omitempty"`
	Google struct {
		APIKey string `json:"apiKey"`
		Model  string `json:"model"`
	} `json:"google"`
	OpenAI struct {
		APIKey string `json:"apiKey"`
		Model  string `json:"model"`
	} `json:"openai"`
	Anthropic struct {
		APIKey string `json:"apiKey"`
		Model  string `json:"model"`
	} `json:"anthropic"`
}

func LoadProviderConfig() (ProviderConfig, error) {
	var cfg ProviderConfig
	b, err := os.ReadFile(ConfigFile())
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func SaveProviderConfig(cfg ProviderConfig) error {
	if err := os.MkdirAll(RuntimeDir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(ConfigFile(), b, 0o600)
}

func ResolveGoogleAPIKey() (string, error) {
	if v := os.Getenv("GOOGLE_API_KEY"); v != "" {
		return v, nil
	}
	cfg, err := LoadProviderConfig()
	if err != nil {
		return "", err
	}
	if cfg.Provider != "google" {
		return "", errors.New("active provider is not google; run 'adkbot onboard' to configure google")
	}
	if cfg.Google.APIKey == "" {
		return "", errors.New("google API key is not configured; run 'adkbot onboard'")
	}
	return cfg.Google.APIKey, nil
}

func ResolveGoogleModel() string {
	if v := os.Getenv("ADKBOT_MODEL"); v != "" {
		return v
	}
	cfg, err := LoadProviderConfig()
	if err != nil {
		return DefaultModel
	}
	if cfg.Provider != "google" {
		return DefaultModel
	}
	if cfg.Google.Model != "" {
		return cfg.Google.Model
	}
	return DefaultModel
}

func ResolveCloudinaryURL() (string, error) {
	if v := os.Getenv("CLOUDINARY_URL"); v != "" {
		return v, nil
	}
	cfg, err := LoadProviderConfig()
	if err != nil {
		return "", err
	}
	if cfg.Cloudinary.URL != "" {
		return cfg.Cloudinary.URL, nil
	}
	if cfg.Cloudinary.CloudName != "" && cfg.Cloudinary.APIKey != "" && cfg.Cloudinary.APISecret != "" {
		return fmt.Sprintf("cloudinary://%s:%s@%s", cfg.Cloudinary.APIKey, cfg.Cloudinary.APISecret, cfg.Cloudinary.CloudName), nil
	}
	return "", errors.New("cloudinary credentials are not configured; set CLOUDINARY_URL or run 'adkbot onboard'")
}
