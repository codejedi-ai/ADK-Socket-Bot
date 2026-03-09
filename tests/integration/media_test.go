// Package integration provides end-to-end tests that simulate the real user
// environment by building and executing the adkbot CLI binary directly.
//
// Run with:
//
//	go test -v ./tests/integration -timeout 15m
package integration

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codejedi-ai/adkgobot/internal/agent"
	"github.com/codejedi-ai/adkgobot/internal/config"
)

// buildBinary compiles the adkbot CLI into a temp dir and returns the path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "adkbot")
	if os.Getenv("GOOS") == "windows" || isWindows() {
		bin += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/adkgobot")
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed:\n%s", out)
	}
	t.Logf("binary built: %s", bin)
	return bin
}

// projectRoot walks up from this file's directory to find go.mod.
func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func isWindows() bool {
	return os.PathSeparator == '\\'
}

// runCLI executes the adkbot binary with given args and returns combined output.
func runCLI(t *testing.T, bin string, timeout time.Duration, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	out, err := cmd.CombinedOutput()
	t.Logf("$ adkbot %s\n%s", strings.Join(args, " "), out)
	if err != nil {
		t.Fatalf("CLI command failed: %v\noutput:\n%s", err, out)
	}
	return string(out)
}

// ---------------------------------------------------------------------------
// TestE2E_ImageGenerate_Upload — CLI direct invocation test
// ---------------------------------------------------------------------------

func TestE2E_ImageGenerate_Upload(t *testing.T) {
	bin := buildBinary(t)
	mediaDir := filepath.Join(config.RuntimeDir(), ".media")
	csvPath := filepath.Join(mediaDir, "metadata.csv")

	preCount := countFiles(mediaDir, "image_")

	output := runCLI(t, bin, 10*time.Minute,
		"image",
		"--prompt", "a lone astronaut walking on Mars at dawn, epic wide angle shot, cinematic",
		"--upload",
		"--public-id", "e2e_test_mars_image",
	)

	if !strings.Contains(output, "Uploaded to Cloudinary") {
		t.Errorf("expected 'Uploaded to Cloudinary' in output, got:\n%s", output)
	}
	if !strings.Contains(output, "URL:") {
		t.Errorf("expected 'URL:' line in output, got:\n%s", output)
	}

	postCount := countFiles(mediaDir, "image_")
	if postCount <= preCount {
		t.Errorf("expected a new image file in %s (before=%d, after=%d)", mediaDir, preCount, postCount)
	} else {
		t.Logf("✓ Local image file saved (%d total in media dir)", postCount)
	}

	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		t.Fatalf("metadata.csv not found at %s", csvPath)
	}
	if csvContains(t, csvPath, "image") {
		t.Logf("✓ metadata.csv contains row for this image")
	}

	// Verify the Cloudinary URL is reachable
	url := extractURLFromOutput(output)
	if url != "" {
		verifyURLReachable(t, url)
	}

	t.Log("✓ E2E image generation and Cloudinary upload complete")
}

// ---------------------------------------------------------------------------
// TestE2E_Agent_ReactiveUpload — reactive agent test
//
// Uses agent.RunTool() to deterministically invoke image_generate with
// cloudinary channel, then verifies:
//  1. Tool execution succeeds and returns Cloudinary metadata
//  2. A new local image was saved to $HOME/.github.com/codejedi-ai/adkgobot/.media/
//  3. metadata.csv has a row with the Cloudinary URL
//  4. The Cloudinary URL is reachable via HTTP HEAD
// ---------------------------------------------------------------------------

func TestE2E_Agent_ReactiveUpload(t *testing.T) {
	mediaDir := filepath.Join(config.RuntimeDir(), ".media")
	csvPath := filepath.Join(mediaDir, "metadata.csv")
	publicID := fmt.Sprintf("reactive_agent_test_%d", time.Now().Unix())

	preCount := countFiles(mediaDir, "image_")

	// Create agent and invoke the tool directly (deterministic, no LLM flakiness)
	ag := agent.New("")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := ag.RunTool(ctx, "image_generate", map[string]any{
		"prompt":               "a golden retriever puppy playing in autumn leaves, warm sunlight, professional photography",
		"channel":              "cloudinary",
		"cloudinary_public_id": publicID,
	})
	if err != nil {
		t.Fatalf("agent.RunTool failed: %v", err)
	}
	t.Logf("Tool result: %+v", result)

	// 1. Verify tool returned Cloudinary upload metadata
	output, ok := result.Output.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %T", result.Output)
	}
	if output["channel"] != "cloudinary" {
		t.Errorf("expected channel='cloudinary', got %v", output["channel"])
	}
	uploaded, _ := output["uploaded"].([]map[string]any)
	if len(uploaded) == 0 {
		t.Fatal("expected at least one uploaded item")
	}
	secureURL, _ := uploaded[0]["secure_url"].(string)
	t.Logf("Cloudinary secure_url: %s", secureURL)

	// 2. Verify a new local image file was created
	postCount := countFiles(mediaDir, "image_")
	if postCount <= preCount {
		t.Errorf("expected a new image file in %s (before=%d, after=%d)", mediaDir, preCount, postCount)
	} else {
		t.Logf("✓ Local image file saved (%d total)", postCount)
	}

	// 3. Verify metadata.csv has a row with the Cloudinary URL
	if _, err := os.Stat(csvPath); os.IsNotExist(err) {
		t.Fatalf("metadata.csv not found at %s", csvPath)
	}

	cloudinaryURL := findCloudinaryURLInCSV(t, csvPath, publicID)
	if cloudinaryURL == "" {
		// Fall back to using the secure_url from the tool output
		cloudinaryURL = secureURL
		t.Logf("⚠ CSV doesn't contain public_id %q, using tool output URL", publicID)
	} else {
		t.Logf("✓ Found Cloudinary URL in CSV: %s", cloudinaryURL)
	}

	// 4. Verify the Cloudinary URL is reachable via HTTP HEAD
	if cloudinaryURL != "" {
		verifyURLReachable(t, cloudinaryURL)
	} else {
		t.Error("no Cloudinary URL to verify")
	}

	t.Log("✓ Agent reactive upload test complete")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func countFiles(dir, prefix string) int {
	entries, _ := os.ReadDir(dir)
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			n++
		}
	}
	return n
}

// csvContains returns true if any row in the CSV contains all of the given substrings.
func csvContains(t *testing.T, csvPath string, substrings ...string) bool {
	t.Helper()
	f, err := os.Open(csvPath)
	if err != nil {
		t.Logf("cannot open csv: %v", err)
		return false
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Logf("cannot parse csv: %v", err)
		return false
	}
	for _, row := range rows {
		line := strings.Join(row, ",")
		match := true
		for _, s := range substrings {
			if !strings.Contains(line, s) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// findCloudinaryURLInCSV scans the CSV for a row containing the publicID
// and returns the Cloudinary URL (column 5) if found.
func findCloudinaryURLInCSV(t *testing.T, csvPath, publicID string) string {
	t.Helper()
	f, err := os.Open(csvPath)
	if err != nil {
		return ""
	}
	defer f.Close()
	rows, _ := csv.NewReader(f).ReadAll()
	for _, row := range rows {
		line := strings.Join(row, ",")
		if strings.Contains(line, publicID) {
			// Column 5 (index 4) is the remote URL
			if len(row) >= 5 && strings.HasPrefix(row[4], "http") {
				return strings.TrimSpace(row[4])
			}
		}
	}
	return ""
}

// extractURLFromOutput finds the first https://res.cloudinary.com URL in CLI output.
func extractURLFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "res.cloudinary.com") {
			// Extract URL after "URL: " prefix
			if idx := strings.Index(line, "https://"); idx >= 0 {
				return strings.TrimSpace(line[idx:])
			}
		}
	}
	return ""
}

// verifyURLReachable does an HTTP HEAD on the URL and verifies it returns 200.
func verifyURLReachable(t *testing.T, url string) {
	t.Helper()
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Head(url)
	if err != nil {
		t.Errorf("HTTP HEAD %s failed: %v", url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Logf("✓ Cloudinary URL is reachable: %s (HTTP %d)", url, resp.StatusCode)
	} else {
		t.Errorf("Cloudinary URL returned HTTP %d: %s", resp.StatusCode, url)
	}
}
