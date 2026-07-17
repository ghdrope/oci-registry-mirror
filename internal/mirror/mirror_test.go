/*
Copyright 2026 Pedro Cozinheiro.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mirror

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

// TestImageExists verifies the behavior of ImageExists depending on Skopeo returns.
func TestImageExists(t *testing.T) {
	tests := []struct {
		name         string
		image        string
		creds        string
		fakeExitCode string
		expectExists bool
		expectError  bool
	}{
		{
			name:         "image exists in target registry",
			image:        "docker://harbor/ubuntu:22.04",
			creds:        "user:pass",
			fakeExitCode: "0",
			expectExists: true,
			expectError:  false,
		},
		{
			name:         "image does not exist in target (skopeo exit code 1)",
			image:        "docker://harbor/ubuntu:99.99",
			creds:        "user:pass",
			fakeExitCode: "1",
			expectExists: false,
			expectError:  false,
		},
		{
			name:         "critical os error occurred (skopeo execution blocks)",
			image:        "docker://harbor/ubuntu:22.04",
			creds:        "user:pass",
			fakeExitCode: "OS_ERR",
			expectExists: false,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldExec := execCommand
			defer func() { execCommand = oldExec }()

			execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				// If we want to simulate a critical OS level error (binary missing, permission denied),
				// we pass a completely invalid binary path. This causes cmd.Run() to fail during Start()
				// with a system error instead of an ExitError.
				if tt.fakeExitCode == "OS_ERR" {
					return exec.CommandContext(ctx, "/nonexistent/binary/path/to/force/error")
				}

				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--")
				cmd.Env = append(os.Environ(),
					"WANT_HELPER_PROCESS=1",
					"SIMULATED_EXIT_CODE="+tt.fakeExitCode,
				)
				return cmd
			}

			exists, err := ImageExists(context.Background(), tt.image, tt.creds)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected execution error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if exists != tt.expectExists {
				t.Errorf("unexpected exists outcome: got %v, want %v", exists, tt.expectExists)
			}
		})
	}
}

// TestCopyImage verifies the wrapper handling of Skopeo copies.
func TestCopyImage(t *testing.T) {
	tests := []struct {
		name         string
		fakeExitCode string
		expectError  bool
	}{
		{
			name:         "successful copy run",
			fakeExitCode: "0",
			expectError:  false,
		},
		{
			name:         "failed copy run",
			fakeExitCode: "1",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldExec := execCommand
			defer func() { execCommand = oldExec }()

			execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--")
				cmd.Env = append(os.Environ(),
					"WANT_HELPER_PROCESS=1",
					"SIMULATED_EXIT_CODE="+tt.fakeExitCode,
				)
				return cmd
			}

			err := CopyImage(context.Background(), "docker://src:tag", "docker://dst:tag", "user:pass")

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// TestHelperProcess is not a real test. It is used to simulate Skopeo command
// execution returns inside tests hermetically.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("WANT_HELPER_PROCESS") != "1" {
		return
	}

	if os.Getenv("SIMULATED_EXIT_CODE") == "1" {
		os.Exit(1)
	}

	os.Exit(0)
}
