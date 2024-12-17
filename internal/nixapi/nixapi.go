package nixapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/log"
)

// PackageInfo represents package metadata from flake output
type PackageInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type PackageDisplay struct {
	Name string
	Key  string
}

// FlakeOutput represents the full nix flake show output
type FlakeOutput struct {
	Packages map[string]map[string]PackageInfo `json:"packages"`
}

// Client handles Nix operations
type Client struct {
	system  string
	timeout time.Duration
	workDir string
}

// NewClient creates a new Nix API client
func NewClient() *Client {
	// Create temporary work directory
	workDir, err := os.MkdirTemp("", "nixtea-*")
	if err != nil {
		log.Error("Failed to create work directory", "error", err)
		// Fall back to /tmp if we can't create our own
		workDir = "/tmp"
	}

	return &Client{
		system:  getCurrentSystem(),
		timeout: 30 * time.Second,
		workDir: workDir,
	}
}

// Close cleans up resources
func (c *Client) Close() error {
	if c.workDir != "/tmp" {
		return os.RemoveAll(c.workDir)
	}
	return nil
}

func getCurrentSystem() string {
	var nixArch, nixOS string

	switch runtime.GOARCH {
	case "amd64":
		nixArch = "x86_64"
	case "arm64":
		nixArch = "aarch64"
	default:
		nixArch = runtime.GOARCH
	}

	switch runtime.GOOS {
	case "darwin":
		nixOS = "darwin"
	case "linux":
		nixOS = "linux"
	default:
		nixOS = runtime.GOOS
	}

	return fmt.Sprintf("%s-%s", nixArch, nixOS)
}

// GetSystemPackages retrieves and filters packages for the current system
func (c *Client) GetSystemPackages(repoURL string) (map[string]PackageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Create command with context
	cmd := exec.CommandContext(ctx, "nix", "flake", "show", "--no-write-lock-file", "--json", repoURL)

	// Create a channel for the result
	type result struct {
		packages map[string]PackageInfo
		err      error
	}

	resultChan := make(chan result, 1)

	// Run the command in a goroutine
	go func() {
		output, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				resultChan <- result{nil, fmt.Errorf("operation timed out after %v", c.timeout)}
				return
			}
			resultChan <- result{nil, fmt.Errorf("failed to run nix flake show: %w\noutput: %s", err, string(output))}
			return
		}

		// Find the actual JSON content by looking for the first '{'
		outputStr := string(output)
		jsonStart := strings.Index(outputStr, "{")
		if jsonStart == -1 {
			resultChan <- result{nil, fmt.Errorf("no JSON found in output: %s", outputStr)}
			return
		}

		// Parse only the JSON portion
		var flakeOutput FlakeOutput
		if err := json.Unmarshal([]byte(outputStr[jsonStart:]), &flakeOutput); err != nil {
			resultChan <- result{nil, fmt.Errorf("failed to parse JSON output: %w\nraw output: %s", err, outputStr)}
			return
		}

		// Get packages for current system
		systemPackages := flakeOutput.Packages[c.system]
		if systemPackages == nil {
			resultChan <- result{make(map[string]PackageInfo), nil}
			return
		}

		resultChan <- result{systemPackages, nil}
	}()

	// Wait for either the result or context cancellation
	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("operation timed out after %v", c.timeout)
		}
		return nil, ctx.Err()
	case res := <-resultChan:
		return res.packages, res.err
	}
}

func (c *Client) GetFormattedPackages(repoURL string) ([]PackageDisplay, error) {
	packages, err := c.GetSystemPackages(repoURL)
	if err != nil {
		return nil, err
	}

	var displayPkgs []PackageDisplay
	for key, pkg := range packages {
		displayPkgs = append(displayPkgs, PackageDisplay{
			Name: pkg.Name,
			Key:  key,
		})
	}
	return displayPkgs, nil
}

func (c *Client) UpdateFlake(repoURL string) error {
	log.Info("Updating flake", "repo_url", repoURL)

	// Create a unique working directory for this update
	updateDir := filepath.Join(c.workDir, fmt.Sprintf("update-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(updateDir, 0755); err != nil {
		return fmt.Errorf("failed to create update directory: %w", err)
	}
	defer os.RemoveAll(updateDir)
	fmt.Println("updateDir", updateDir)

	// Step 1: Clone/fetch the latest repository content
	cmd := exec.Command("nix", "flake", "clone", "--dest", updateDir, repoURL)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to clone flake: %w\noutput: %s", err, string(output))
	}

	// Step 2: Update the flake's inputs
	cmd = exec.Command("nix", "flake", "update",
		"--commit-lock-file",
		"--experimental-features", "nix-command flakes",
		"--option", "warn-dirty", "false",
		"--flake", updateDir)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to update flake: %w\noutput: %s", err, string(output))
	}

	// Step 3: Verify the flake can be evaluated
	cmd = exec.Command("nix", "flake", "check",
		"--no-write-lock-file",
		"--experimental-features", "nix-command flakes",
		updateDir)

	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("flake check failed: %w\noutput: %s", err, string(output))
	}

	log.Info("Flake updated successfully")
	return nil
}

// BuildResult represents the output of building a package
type BuildResult struct {
	StorePath  string
	BinaryPath string
}

// BuildPkg builds a package and returns the path to its binary
func (c *Client) BuildPkg(repoURL, pkgKey string) (*BuildResult, error) {
	log.Info("Building package", "repo", repoURL, "key", pkgKey)

	fullPkgURL := fmt.Sprintf("%s#%s", repoURL, pkgKey)
	buildCmd := exec.Command("nix", "build", "--no-write-lock-file", "--print-out-paths", fullPkgURL)

	outBytes, err := buildCmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("build failed: %w\nOutput: %s", err, string(outBytes))
	}

	storePath := strings.TrimSpace(string(outBytes))
	if storePath == "" {
		return nil, fmt.Errorf("no store path returned from build")
	}
	log.Info("Package built successfully", "storePath", storePath)

	// Find the binary
	binDir := filepath.Join(storePath, "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read bin directory: %w", err)
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no binaries found in %s", binDir)
	}

	if len(entries) > 1 {
		var binNames []string
		for _, entry := range entries {
			binNames = append(binNames, entry.Name())
		}
		return nil, fmt.Errorf("multiple binaries found in %s: %v", binDir, binNames)
	}

	binaryPath := filepath.Join(binDir, entries[0].Name())
	log.Info("Found binary", "path", binaryPath)

	return &BuildResult{
		StorePath:  storePath,
		BinaryPath: binaryPath,
	}, nil
}

// BuildPackage handles the building of a package with proper error handling and logging
func (c *Client) BuildPackage(repoURL, pkgKey string) (*BuildResult, error) {
	log.Info("Building package", "repo", repoURL, "key", pkgKey)

	// Build the package using the remote flake directly
	cmd := exec.Command("nix", "build", "--no-write-lock-file", "--print-out-paths",
		fmt.Sprintf("%s#%s", repoURL, pkgKey))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("build failed: %w\nOutput: %s", err, string(output))
	}

	storePath := strings.TrimSpace(string(output))
	log.Info("Build successful", "storePath", storePath)
	if storePath == "" {
		return nil, fmt.Errorf("empty store path returned")
	}

	// First check if there's a binary in the store path directly
	if stat, err := os.Stat(storePath); err == nil && !stat.IsDir() {
		return &BuildResult{
			StorePath:  storePath,
			BinaryPath: storePath,
		}, nil
	}

	// If not, then check bin directory
	binDir := filepath.Join(storePath, "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		// If no bin directory, the store path itself might be the binary
		return &BuildResult{
			StorePath:  storePath,
			BinaryPath: storePath,
		}, nil
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("no binaries found in %s", binDir)
	}

	binaryPath := filepath.Join(binDir, entries[0].Name())
	return &BuildResult{
		StorePath:  storePath,
		BinaryPath: binaryPath,
	}, nil

}
