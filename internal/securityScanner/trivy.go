package securityscanner

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

var execCommandContext = exec.CommandContext
var lookPath = exec.LookPath

// TrivyScanResult represents the minimal JSON structure returned by Trivy
// that we need to inspect to determine if vulnerabilities were found.
type TrivyScanResult struct {
	Results []struct {
		Vulnerabilities []interface{} `json:"Vulnerabilities"`
	} `json:"Results"`
}

// CheckVulnerabilities invokes the Trivy CLI to scan a target image for
// HIGH and CRITICAL vulnerabilities. It returns true if any vulnerabilities are found.
func CheckVulnerabilities(ctx context.Context, image string) (bool, error) {
	// Ensure trivy binary is present in the system path
	if _, err := lookPath("trivy"); err != nil {
		return false, fmt.Errorf("trivy binary not found in PATH: %w", err)
	}

	args := []string{
		"image",
		"--severity", "HIGH,CRITICAL",
		"--no-progress",
		"--format", "json",
		"--timeout", "10m",
		image,
	}

	// Execute command bound strictly to the incoming context life cycle
	cmd := execCommandContext(ctx, "trivy", args...)

	// Trivy exists with non-zero if errors occur or vulnerabilities are detected.
	// We safely verify if we generated a valid JSON payload regardless of exit status code.
	outputBytes, _ := cmd.Output()

	var scanResult TrivyScanResult
	if jsonErr := json.Unmarshal(outputBytes, &scanResult); jsonErr != nil {
		return false, fmt.Errorf("failed parsing trivy JSON output: %w", jsonErr)
	}

	// Iterate through findings to verify if the slice contains any structural vulnerability elements
	for _, result := range scanResult.Results {
		if len(result.Vulnerabilities) > 0 {
			return true, nil
		}
	}

	return false, nil
}
