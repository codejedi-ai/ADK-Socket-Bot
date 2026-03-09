package genai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var flashModelPattern = regexp.MustCompile(`^gemini-(\d+)\.(\d+)-flash`) // gemini-2.5-flash*.
var proModelPattern = regexp.MustCompile(`^gemini-(\d+)\.(\d+)-pro`)     // gemini-2.5-pro*.

// DiscoverNewestFlashModel returns the highest-version active Flash model
// that supports generateContent.
func DiscoverNewestFlashModel(ctx context.Context, apiKey string) (string, error) {
	return discoverNewestByFamily(ctx, apiKey, flashModelPattern)
}

// CheckAPIKeyHealth returns nil when the API key is valid and can access model listing.
func CheckAPIKeyHealth(ctx context.Context, apiKey string) error {
	_, err := listGenerateContentModels(ctx, apiKey)
	return err
}

// DiscoverNewestProModel returns the highest-version active Pro model
// that supports generateContent.
func DiscoverNewestProModel(ctx context.Context, apiKey string) (string, error) {
	return discoverNewestByFamily(ctx, apiKey, proModelPattern)
}

func discoverNewestByFamily(ctx context.Context, apiKey string, pat *regexp.Regexp) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", errors.New("api key is required")
	}
	models, err := listGenerateContentModels(ctx, apiKey)
	if err != nil {
		return "", err
	}
	matches := make([]string, 0)
	for _, m := range models {
		if pat.MatchString(m) {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return "", errors.New("no active model found for requested family")
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return modelMorePreferred(matches[i], matches[j], pat)
	})
	return matches[0], nil
}

func discoverBestGenerateContentModel(ctx context.Context, apiKey string) (string, error) {
	flash, err := DiscoverNewestFlashModel(ctx, apiKey)
	if err == nil && flash != "" {
		return flash, nil
	}
	models, err := listGenerateContentModels(ctx, apiKey)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "", errors.New("no active gemini models support generateContent")
	}
	sort.Strings(models)
	return models[0], nil
}

func listGenerateContentModels(ctx context.Context, apiKey string) ([]string, error) {
	hc := http.Client{Timeout: 20 * time.Second}
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models?key=%s", apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model list error (%d): %s", resp.StatusCode, string(body))
	}
	var out struct {
		Models []struct {
			Name                       string   `json:"name"`
			SupportedGenerationMethods []string `json:"supportedGenerationMethods"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}

	models := make([]string, 0)
	for _, m := range out.Models {
		if !supportsMethod(m.SupportedGenerationMethods, "generateContent") {
			continue
		}
		name := strings.TrimPrefix(m.Name, "models/")
		if strings.HasPrefix(name, "gemini") {
			models = append(models, name)
		}
	}
	return models, nil
}

func modelMorePreferred(a, b string, pat *regexp.Regexp) bool {
	// Prefer higher major.minor version; for same version prefer non-preview.
	amaj, amin := modelVersion(a, pat)
	bmaj, bmin := modelVersion(b, pat)
	if amaj != bmaj {
		return amaj > bmaj
	}
	if amin != bmin {
		return amin > bmin
	}
	aPreview := strings.Contains(a, "preview")
	bPreview := strings.Contains(b, "preview")
	if aPreview != bPreview {
		return !aPreview
	}
	return a < b
}

func flashVersion(name string) (int, int) {
	return modelVersion(name, flashModelPattern)
}

func modelVersion(name string, pat *regexp.Regexp) (int, int) {
	m := pat.FindStringSubmatch(name)
	if len(m) != 3 {
		return 0, 0
	}
	maj := 0
	min := 0
	_, _ = fmt.Sscanf(m[1], "%d", &maj)
	_, _ = fmt.Sscanf(m[2], "%d", &min)
	return maj, min
}
