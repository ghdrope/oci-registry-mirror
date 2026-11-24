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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"go.uber.org/zap"
)

// fakeSkopeo makes execCommand run TestHelperProcess instead of skopeo, which
// exits with exitCode and writes stderr. The skopeo arguments are stored in
// *gotArgs. exitCode "OS_ERR" simulates a binary that can't be started.
func fakeSkopeo(t *testing.T, exitCode, stderr string, gotArgs *[]string) {
	t.Helper()
	oldExec := execCommand
	t.Cleanup(func() { execCommand = oldExec })

	execCommand = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "skopeo" {
			t.Fatalf("unexpected binary %q; want skopeo", name)
		}
		if gotArgs != nil {
			*gotArgs = args
		}

		// A missing binary makes cmd.Run() fail during Start() with a system
		// error instead of an ExitError.
		if exitCode == "OS_ERR" {
			return exec.CommandContext(ctx, "/nonexistent/binary/path/to/force/error")
		}

		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=TestHelperProcess", "--")
		cmd.Env = append(os.Environ(),
			"WANT_HELPER_PROCESS=1",
			"SIMULATED_EXIT_CODE="+exitCode,
			"SIMULATED_STDERR="+stderr,
		)
		return cmd
	}
}

// TestImageExists verifies the behavior of ImageExists depending on Skopeo returns.
func TestImageExists(t *testing.T) {
	tests := []struct {
		name         string
		exitCode     string
		stderr       string
		expectExists bool
		expectError  string
	}{
		{
			name:         "image exists in target registry",
			exitCode:     "0",
			expectExists: true,
		},
		{
			name:     "missing tag (manifest unknown)",
			exitCode: "1",
			stderr:   `time="..." level=fatal msg="Error parsing image name: reading manifest 99.99 in harbor/ubuntu: manifest unknown"`,
		},
		{
			name:     "missing repository (name unknown)",
			exitCode: "1",
			stderr:   "reading manifest 1.0 in registry/missing: name unknown: repository name not known to registry",
		},
		{
			name:     "missing artifact (Harbor not found)",
			exitCode: "1",
			stderr:   "reading manifest 1.0 in harbor/project/x: unknown: artifact project/x:1.0 not found",
		},
		{
			name:        "wrong credentials are an error, not a missing image",
			exitCode:    "1",
			stderr:      "reading manifest 22.04 in harbor/ubuntu: unauthorized: authentication required",
			expectError: "unauthorized: authentication required",
		},
		{
			name:        "failure without stderr",
			exitCode:    "1",
			expectError: "skopeo inspect failed: exit status 1",
		},
		{
			name:        "critical os error occurred (skopeo execution blocks)",
			exitCode:    "OS_ERR",
			expectError: "no such file or directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var args []string
			fakeSkopeo(t, tt.exitCode, tt.stderr, &args)

			exists, err := ImageExists(context.Background(), "docker://harbor/ubuntu:22.04", "user:pass")

			if tt.expectError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.expectError) {
					t.Fatalf("ImageExists() error = %v; want containing %q", err, tt.expectError)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if exists != tt.expectExists {
				t.Errorf("unexpected exists outcome: got %v, want %v", exists, tt.expectExists)
			}

			want := []string{"inspect", "--raw", "--retry-times", retryTimes, "--creds", "user:pass", "docker://harbor/ubuntu:22.04"}
			if !reflect.DeepEqual(args, want) {
				t.Errorf("skopeo args = %v; want %v", args, want)
			}
		})
	}
}

// TestCopyImage verifies the wrapper handling of Skopeo copies, and that
// every platform of multi-arch images is copied.
func TestCopyImage(t *testing.T) {
	tests := []struct {
		name        string
		exitCode    string
		expectError bool
	}{
		{name: "successful copy run", exitCode: "0"},
		{name: "failed copy run", exitCode: "1", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var args []string
			fakeSkopeo(t, tt.exitCode, "", &args)

			err := CopyImage(context.Background(), "docker://src:tag", "docker://dst:tag", "user:pass")

			if (err != nil) != tt.expectError {
				t.Fatalf("CopyImage() error = %v; want error %v", err, tt.expectError)
			}

			want := []string{"copy", "--all", "--retry-times", retryTimes, "--dest-creds", "user:pass", "docker://src:tag", "docker://dst:tag"}
			if !reflect.DeepEqual(args, want) {
				t.Errorf("skopeo args = %v; want %v", args, want)
			}
		})
	}
}

// fakeRegistry replaces the skopeo calls used by Run. existing lists the
// destination references already present; copies records every copy as
// "source -> destination".
type fakeRegistry struct {
	existing   map[string]bool
	inspectErr error
	copyErr    error
	copies     []string
	inspected  []string
}

func (f *fakeRegistry) install(t *testing.T) {
	t.Helper()
	oldExists, oldCopy := imageExists, copyImage
	t.Cleanup(func() { imageExists, copyImage = oldExists, oldCopy })

	imageExists = func(_ context.Context, image, creds string) (bool, error) {
		if creds != "user:pass" {
			t.Errorf("imageExists creds = %q; want user:pass", creds)
		}
		f.inspected = append(f.inspected, image)
		return f.existing[image], f.inspectErr
	}
	copyImage = func(_ context.Context, source, destination, creds string) error {
		if creds != "user:pass" {
			t.Errorf("copyImage creds = %q; want user:pass", creds)
		}
		f.copies = append(f.copies, source+" -> "+destination)
		return f.copyErr
	}
}

var (
	golang = config.ImageEntry{Name: "golang", Source: "docker.io/library/golang", Destination: "registry/golang", Tag: "1.27"}
	alpine = config.ImageEntry{Name: "alpine", Source: "docker.io/library/alpine", Destination: "registry/alpine", Tag: "3.22"}
)

// TestRunMirrorsMissingImages verifies missing images are copied to
// "latest" first and to their own tag last, and present images are skipped.
func TestRunMirrorsMissingImages(t *testing.T) {
	reg := &fakeRegistry{existing: map[string]bool{"docker://registry/alpine:3.22": true}}
	reg.install(t)

	summary, err := Run(context.Background(), zap.NewNop(), []config.ImageEntry{golang, alpine}, "user:pass", false)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	wantCopies := []string{
		"docker://docker.io/library/golang:1.27 -> docker://registry/golang:latest",
		"docker://docker.io/library/golang:1.27 -> docker://registry/golang:1.27",
	}
	if !reflect.DeepEqual(reg.copies, wantCopies) {
		t.Errorf("copies = %v; want %v", reg.copies, wantCopies)
	}
	if want := (Summary{Total: 2, Skipped: 1, Mirrored: 1}); summary != want {
		t.Errorf("summary = %+v; want %+v", summary, want)
	}
}

// TestRunLatestTagIsAlwaysRefreshed verifies an image tagged "latest" is
// copied on every run (once, to "latest"), since that tag moves.
func TestRunLatestTagIsAlwaysRefreshed(t *testing.T) {
	reg := &fakeRegistry{existing: map[string]bool{"docker://registry/nginx:latest": true}}
	reg.install(t)
	nginx := config.ImageEntry{Name: "nginx", Source: "docker.io/library/nginx", Destination: "registry/nginx", Tag: "latest"}

	summary, err := Run(context.Background(), zap.NewNop(), []config.ImageEntry{nginx}, "user:pass", false)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(reg.inspected) != 0 {
		t.Errorf("inspected = %v; an image tagged latest must not be skipped", reg.inspected)
	}
	want := []string{"docker://docker.io/library/nginx:latest -> docker://registry/nginx:latest"}
	if !reflect.DeepEqual(reg.copies, want) {
		t.Errorf("copies = %v; want %v", reg.copies, want)
	}
	if summary.Mirrored != 1 || summary.Skipped != 0 {
		t.Errorf("summary = %+v; want 1 mirrored, 0 skipped", summary)
	}
}

// TestRunDryRun verifies dry-run still inspects the destination (which
// checks the credentials) but copies nothing.
func TestRunDryRun(t *testing.T) {
	reg := &fakeRegistry{}
	reg.install(t)

	summary, err := Run(context.Background(), zap.NewNop(), []config.ImageEntry{golang}, "user:pass", true)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(reg.copies) != 0 {
		t.Errorf("dry-run copied %v", reg.copies)
	}
	if !slices.Equal(reg.inspected, []string{"docker://registry/golang:1.27"}) {
		t.Errorf("inspected = %v; want the explicit destination tag", reg.inspected)
	}
	if want := (Summary{Total: 1, Mirrored: 1}); summary != want {
		t.Errorf("summary = %+v; want %+v", summary, want)
	}
}

// TestRunErrors verifies inspect and copy failures stop the run, and that a
// failed copy never reaches the explicit tag (so the next run retries it).
func TestRunErrors(t *testing.T) {
	t.Run("inspect failure", func(t *testing.T) {
		reg := &fakeRegistry{inspectErr: errors.New("unauthorized")}
		reg.install(t)

		_, err := Run(context.Background(), zap.NewNop(), []config.ImageEntry{golang}, "user:pass", false)
		if err == nil || !strings.Contains(err.Error(), "failed checking image existence") {
			t.Fatalf("Run() error = %v; want an inspect error", err)
		}
		if len(reg.copies) != 0 {
			t.Errorf("copies = %v; want none after an inspect failure", reg.copies)
		}
	})

	t.Run("copy failure", func(t *testing.T) {
		reg := &fakeRegistry{copyErr: errors.New("boom")}
		reg.install(t)

		summary, err := Run(context.Background(), zap.NewNop(), []config.ImageEntry{golang, alpine}, "user:pass", false)
		if err == nil || !strings.Contains(err.Error(), "skopeo copy failed for registry/golang:latest") {
			t.Fatalf("Run() error = %v; want a copy error", err)
		}
		if len(reg.copies) != 1 {
			t.Errorf("copies = %v; want the run to stop at the first failure", reg.copies)
		}
		if summary.Mirrored != 0 {
			t.Errorf("summary = %+v; want nothing mirrored", summary)
		}
	})
}

// TestRunCancelled verifies a cancelled context stops the run before any
// registry call.
func TestRunCancelled(t *testing.T) {
	reg := &fakeRegistry{}
	reg.install(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Run(ctx, zap.NewNop(), []config.ImageEntry{golang}, "user:pass", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v; want context.Canceled", err)
	}
	if len(reg.inspected)+len(reg.copies) != 0 {
		t.Errorf("registry was called after cancellation: %v %v", reg.inspected, reg.copies)
	}
}

// TestRunCancelledBetweenCopies verifies the run stops between the copies of
// an image when the context is cancelled.
func TestRunCancelledBetweenCopies(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := &fakeRegistry{}
	reg.install(t)
	copyImage = func(_ context.Context, source, destination, _ string) error {
		reg.copies = append(reg.copies, source+" -> "+destination)
		cancel()
		return nil
	}

	_, err := Run(ctx, zap.NewNop(), []config.ImageEntry{golang}, "user:pass", false)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error = %v; want context.Canceled", err)
	}
	if len(reg.copies) != 1 {
		t.Errorf("copies = %v; want only the first copy", reg.copies)
	}
}

// TestHelperProcess is not a real test. It is used to simulate Skopeo command
// execution returns inside tests hermetically.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("WANT_HELPER_PROCESS") != "1" {
		return
	}

	if stderr := os.Getenv("SIMULATED_STDERR"); stderr != "" {
		_, _ = fmt.Fprintln(os.Stderr, stderr)
	}

	if os.Getenv("SIMULATED_EXIT_CODE") == "1" {
		os.Exit(1)
	}

	os.Exit(0)
}
