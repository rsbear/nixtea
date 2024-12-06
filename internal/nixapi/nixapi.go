package nixapi

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
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
}

// NewClient creates a new Nix API client
func NewClient(system string) *Client {
	return &Client{
		system:  system,
		timeout: 30 * time.Second,
	}
}

// GetSystemPackages retrieves and filters packages for the current system
func (c *Client) GetSystemPackages(repoURL string) (map[string]PackageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	// Create command with context
	cmd := exec.CommandContext(ctx, "nix", "flake", "show", "--json", repoURL)

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
