package securityscanner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"go.uber.org/zap"
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

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Trivy may exit with non-zero when vulnerabilities are detected
	// (--exit-code), so the result is decided by the JSON report. The exit
	// error and stderr are only used to explain a missing or broken report.
	outputBytes, runErr := cmd.Output()

	var scanResult TrivyScanResult
	if jsonErr := json.Unmarshal(outputBytes, &scanResult); jsonErr != nil {
		if runErr != nil {
			if reason := strings.TrimSpace(stderr.String()); reason != "" {
				return false, fmt.Errorf("trivy scan failed: %w: %s", runErr, lastLine(reason))
			}
			return false, fmt.Errorf("trivy scan failed: %w", runErr)
		}
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

// lastLine returns the last line of s: trivy logs progress before the
// actual error.
func lastLine(s string) string {
	lines := strings.Split(s, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// checkVulnerabilities is the scan used by Run. It is a variable so tests
// can replace it.
var checkVulnerabilities = CheckVulnerabilities

// Summary counts the images handled by Run.
type Summary struct {
	Total      int      // images scanned
	Vulnerable int      // images with HIGH or CRITICAL vulnerabilities
	Bypassed   int      // vulnerable images with ignore-severities set
	Failed     []string // vulnerable images without ignore-severities
}

// Run scans the source of every image. All images are scanned before
// deciding, so the summary lists every failing image at once; Run returns
// an error when at least one vulnerable image is not marked with
// ignore-severities.
func Run(ctx context.Context, logger *zap.Logger, images []config.ImageEntry) (Summary, error) {
	var summary Summary

	for _, entry := range images {
		if err := ctx.Err(); err != nil {
			return summary, fmt.Errorf("scan execution cancelled: %w", err)
		}

		summary.Total++
		target := fmt.Sprintf("%s:%s", entry.Source, entry.Tag)

		logger.Info("Scanning image", zap.String("name", entry.Name), zap.String("target", target))

		vulnerable, err := checkVulnerabilities(ctx, target)
		if err != nil {
			return summary, fmt.Errorf("vulnerability scan processing failed for %s: %w", target, err)
		}

		if !vulnerable {
			logger.Info("✅ Image passed vulnerability scanning", zap.String("target", target))
			continue
		}

		summary.Vulnerable++

		if entry.IgnoreSeverities {
			logger.Warn("⚠️ Vulnerabilities found but 'ignore-severities' is set, not blocking",
				zap.String("name", entry.Name),
				zap.String("target", target),
			)
			summary.Bypassed++
			continue
		}

		logger.Error("❌ Image failed the security scan", zap.String("target", target))
		summary.Failed = append(summary.Failed, target)
	}

	if len(summary.Failed) > 0 {
		return summary, fmt.Errorf(
			"🚨 security scan failed: %d target images contain un-ignored HIGH or CRITICAL vulnerabilities: [%s]",
			len(summary.Failed),
			strings.Join(summary.Failed, ", "),
		)
	}

	return summary, nil
}
