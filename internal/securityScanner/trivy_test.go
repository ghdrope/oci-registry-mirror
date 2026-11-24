package securityscanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"go.uber.org/zap"
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

			oldLookPath := lookPath
			defer func() { lookPath = oldLookPath }()

			lookPath = func(file string) (string, error) {
				if file == "trivy" {
					return "/usr/local/bin/trivy", nil
				}
				return "", exec.ErrNotFound
			}

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
	_, _ = fmt.Fprint(os.Stderr, os.Getenv("MOCK_STDERR"))

	// Extract the intentional evaluation status exit code
	var exitCode int
	if _, err := fmt.Sscanf(os.Getenv("MOCK_EXIT_CODE"), "%d", &exitCode); err != nil {
		os.Exit(255)
	}

	os.Exit(exitCode)
}

// fakeTrivy makes the trivy calls run TestHelperProcess, which prints stdout
// and stderr and exits with exitCode.
func fakeTrivy(t *testing.T, stdout, stderr string, exitCode int) {
	t.Helper()
	oldExec, oldLookPath := execCommandContext, lookPath
	t.Cleanup(func() { execCommandContext, lookPath = oldExec, oldLookPath })

	lookPath = func(string) (string, error) { return "/usr/local/bin/trivy", nil }
	execCommandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess")
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("%s=1", HelperProcessCommandKey),
			"MOCK_STDOUT="+stdout,
			"MOCK_STDERR="+stderr,
			fmt.Sprintf("MOCK_EXIT_CODE=%d", exitCode),
		)
		return cmd
	}
}

// TestCheckVulnerabilitiesReportsTrivyError verifies a failed scan without a
// JSON report returns trivy's own error message, not a JSON parse error.
func TestCheckVulnerabilitiesReportsTrivyError(t *testing.T) {
	fakeTrivy(t, "", "2026-01-01T00:00:00Z INFO Need to update DB\n2026-01-01T00:00:01Z FATAL image scan error: unable to find the specified image", 1)

	_, err := CheckVulnerabilities(context.Background(), "docker.io/library/missing:1.0")
	if err == nil {
		t.Fatal("CheckVulnerabilities() error = nil; want an error")
	}
	if !strings.Contains(err.Error(), "unable to find the specified image") || strings.Contains(err.Error(), "Need to update DB") {
		t.Errorf("CheckVulnerabilities() error = %q; want the last stderr line", err)
	}
}

// TestCheckVulnerabilitiesFailureWithoutStderr verifies the exit error is
// returned when trivy fails silently.
func TestCheckVulnerabilitiesFailureWithoutStderr(t *testing.T) {
	fakeTrivy(t, "", "", 3)

	_, err := CheckVulnerabilities(context.Background(), "img:1")
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("CheckVulnerabilities() error = %v; want the exit status", err)
	}
}

// TestCheckVulnerabilitiesInvalidJSONWithSuccess verifies an unexpected
// report is an error even when trivy exits successfully.
func TestCheckVulnerabilitiesInvalidJSONWithSuccess(t *testing.T) {
	fakeTrivy(t, "not json", "", 0)

	_, err := CheckVulnerabilities(context.Background(), "img:1")
	if err == nil || !strings.Contains(err.Error(), "failed parsing trivy JSON output") {
		t.Fatalf("CheckVulnerabilities() error = %v; want a JSON parse error", err)
	}
}

// TestCheckVulnerabilitiesTrivyMissing verifies a missing trivy binary is
// reported before running anything.
func TestCheckVulnerabilitiesTrivyMissing(t *testing.T) {
	oldLookPath := lookPath
	t.Cleanup(func() { lookPath = oldLookPath })
	lookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	_, err := CheckVulnerabilities(context.Background(), "img:1")
	if err == nil || !strings.Contains(err.Error(), "trivy binary not found") {
		t.Fatalf("CheckVulnerabilities() error = %v; want trivy not found", err)
	}
}

// fakeScan replaces the scan used by Run with fixed results per target.
func fakeScan(t *testing.T, results map[string]bool, scanErr error) *[]string {
	t.Helper()
	old := checkVulnerabilities
	t.Cleanup(func() { checkVulnerabilities = old })

	var scanned []string
	checkVulnerabilities = func(_ context.Context, image string) (bool, error) {
		scanned = append(scanned, image)
		return results[image], scanErr
	}
	return &scanned
}

// TestRun verifies every image is scanned, vulnerable images with
// ignore-severities are bypassed, and the others fail the run together.
func TestRun(t *testing.T) {
	scanned := fakeScan(t, map[string]bool{
		"docker.io/library/nginx:1.19": true,
		"docker.io/library/redis:6":    true,
	}, nil)

	images := []config.ImageEntry{
		{Name: "alpine", Source: "docker.io/library/alpine", Tag: "3.22"},
		{Name: "nginx", Source: "docker.io/library/nginx", Tag: "1.19"},
		{Name: "redis", Source: "docker.io/library/redis", Tag: "6", IgnoreSeverities: true},
	}

	summary, err := Run(context.Background(), zap.NewNop(), images)

	if err == nil || !strings.Contains(err.Error(), "1 target images") || !strings.Contains(err.Error(), "docker.io/library/nginx:1.19") {
		t.Fatalf("Run() error = %v; want nginx reported as failing", err)
	}
	want := Summary{Total: 3, Vulnerable: 2, Bypassed: 1, Failed: []string{"docker.io/library/nginx:1.19"}}
	if !reflect.DeepEqual(summary, want) {
		t.Errorf("summary = %+v; want %+v", summary, want)
	}
	if len(*scanned) != 3 {
		t.Errorf("scanned = %v; want every image", *scanned)
	}
}

// TestRunClean verifies a run without vulnerable images succeeds.
func TestRunClean(t *testing.T) {
	fakeScan(t, nil, nil)

	summary, err := Run(context.Background(), zap.NewNop(), []config.ImageEntry{{Source: "alpine", Tag: "3"}})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Total != 1 || summary.Vulnerable != 0 {
		t.Errorf("summary = %+v; want 1 clean image", summary)
	}
}

// TestRunErrors verifies a scan error and a cancelled context stop the run.
func TestRunErrors(t *testing.T) {
	t.Run("scan error", func(t *testing.T) {
		fakeScan(t, nil, errors.New("trivy crashed"))

		_, err := Run(context.Background(), zap.NewNop(), []config.ImageEntry{{Source: "alpine", Tag: "3"}})
		if err == nil || !strings.Contains(err.Error(), "trivy crashed") {
			t.Fatalf("Run() error = %v; want the scan error", err)
		}
	})

	t.Run("cancelled", func(t *testing.T) {
		scanned := fakeScan(t, nil, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := Run(ctx, zap.NewNop(), []config.ImageEntry{{Source: "alpine", Tag: "3"}})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run() error = %v; want context.Canceled", err)
		}
		if len(*scanned) != 0 {
			t.Errorf("scanned = %v; want nothing after cancellation", *scanned)
		}
	})
}
