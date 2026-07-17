package securityscanner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

// HelperProcessCommandKey defines the environment variable flag used to pivot
// the test binary execution path into the simulated Trivy mock behavior.
const HelperProcessCommandKey = "GO_WANT_HELPER_PROCESS"

// TestCheckVulnerabilities executes table-driven test cases validating image scan
// analysis against simulated clean, vulnerable, and malformed Trivy outputs.
func TestCheckVulnerabilities(t *testing.T) {
	tests := []struct {
		name          string
		image         string
		mockStdout    string
		mockExitCode  int
		wantResult    bool
		wantErrResult bool
	}{
		{
			name:  "Clean image with empty vulnerability slices",
			image: "docker.io/library/alpine:latest",
			mockStdout: `{
				"Results": [
					{
						"Target": "alpine:latest",
						"Vulnerabilities": []
					}
				]
			}`,
			mockExitCode:  0,
			wantResult:    false,
			wantErrResult: false,
		},
		{
			name:  "Vulnerable image containing structural findings",
			image: "docker.io/library/nginx:1.19",
			mockStdout: `{
				"Results": [
					{
						"Target": "nginx:1.19",
						"Vulnerabilities": [
							{"VulnerabilityID": "CVE-2026-1234", "Severity": "CRITICAL"}
						]
					}
				]
			}`,
			mockExitCode:  1, // Trivy standard behavior on findings matching metrics bounds
			wantResult:    true,
			wantErrResult: false,
		},
		{
			name:          "Malformed non-JSON output capturing engine crash failure",
			image:         "docker.io/library/broken:latest",
			mockStdout:    "fatal: out of memory tracking layers",
			mockExitCode:  2,
			wantResult:    false,
			wantErrResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			// Overwrite the default exec.Command logic inside the test runtime block.
			// This forces exec.CommandContext calls to point back to our internal helper payload.
			oldExecCommand := execCommandContext
			defer func() { execCommandContext = oldExecCommand }()

			execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				// Safety check: ensure we are intercepting the intended binary target call
				if name != "trivy" {
					t.Fatalf("unexpected binary call detected: expected 'trivy', got '%s'", name)
				}

				// Spawn the current test binary executable as a subprocess execution unit
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
				cmd.Env = append(os.Environ(),
					fmt.Sprintf("%s=1", HelperProcessCommandKey),
					fmt.Sprintf("MOCK_STDOUT=%s", tt.mockStdout),
					fmt.Sprintf("MOCK_EXIT_CODE=%d", tt.mockExitCode),
				)
				return cmd
			}

			got, err := CheckVulnerabilities(ctx, tt.image)

			if (err != nil) != tt.wantErrResult {
				t.Fatalf("CheckVulnerabilities() error status unexpected = %v, wantErr %v", err, tt.wantErrResult)
			}

			if got != tt.wantResult {
				t.Errorf("CheckVulnerabilities() evaluated output = %v, expected %v", got, tt.wantResult)
			}
		})
	}
}

// TestHelperProcess acts as the mock registry controller executable.
// It bypasses the standard testing lifecycle when the matching environment variable is active,
// mimicking standard CLI stdout payloads and standard status codes.
func TestHelperProcess(t *testing.T) {
	if os.Getenv(HelperProcessCommandKey) != "1" {
		return
	}

	// Output the pre-configured mock text/JSON string back to standard output stream channel
	_, _ = fmt.Fprint(os.Stdout, os.Getenv("MOCK_STDOUT"))

	// Extract the intentional evaluation status exit code
	var exitCode int
	if _, err := fmt.Sscanf(os.Getenv("MOCK_EXIT_CODE"), "%d", &exitCode); err != nil {
		os.Exit(255)
	}

	os.Exit(exitCode)
}
