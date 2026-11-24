package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// TestConfigParsing verifies that the images.yaml configuration parsing
// maps correctly into our defined Go structures.
func TestConfigParsing(t *testing.T) {
	tests := []struct {
		name        string
		yamlData    string
		expectError bool
		expected    Config
	}{
		{
			name: "valid configuration with single tag and name",
			yamlData: `images:
  - name: "Ubuntu Base Image"
    source: docker.io/library/ubuntu
    destination: harbor.com/originals/ubuntu
    tag: "22.04"`,
			expectError: false,
			expected: Config{
				Images: []ImageEntry{
					{
						Name:        "Ubuntu Base Image",
						Source:      "docker.io/library/ubuntu",
						Destination: "harbor.com/originals/ubuntu",
						Tag:         "22.04",
					},
				},
			},
		},
		{
			name:        "empty image list is allowed by parser",
			yamlData:    `images: []`,
			expectError: false,
			expected:    Config{Images: []ImageEntry{}},
		},
		{
			name: "malformed yaml structure triggers error",
			yamlData: `images:
  - name: "Broken Image"
    source: docker.io/library/ubuntu
    destination: harbor.com/originals/ubuntu
    tag: "22.04`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg Config
			err := yaml.Unmarshal([]byte(tt.yamlData), &cfg)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error while parsing yaml, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error parsing configuration: %v", err)
			}

			if !reflect.DeepEqual(cfg, tt.expected) {
				t.Errorf("unexpected parsing output:\ngot:  %+v\nwant: %+v", cfg, tt.expected)
			}
		})
	}
}

// writeImagesFile writes content to an images file in a temporary directory
// and returns its path.
func writeImagesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "images.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLoad verifies a valid images file is loaded, including the
// ignore-severities flag.
func TestLoad(t *testing.T) {
	path := writeImagesFile(t, `images:
  - name: golang
    source: docker.io/library/golang
    destination: registry/golang
    tag: "1.27"
    ignore-severities: true
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	want := []ImageEntry{{
		Name:             "golang",
		Source:           "docker.io/library/golang",
		Destination:      "registry/golang",
		Tag:              "1.27",
		IgnoreSeverities: true,
	}}
	if !reflect.DeepEqual(cfg.Images, want) {
		t.Errorf("Load() images = %+v; want %+v", cfg.Images, want)
	}
}

// TestLoadErrors verifies a missing, unreadable, malformed or invalid images
// file is reported.
func TestLoadErrors(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{
			name:    "missing file",
			path:    filepath.Join(t.TempDir(), "missing.yaml"),
			wantErr: "does not exist",
		},
		{
			name:    "directory instead of a file",
			path:    t.TempDir(),
			wantErr: "failed to read",
		},
		{
			name:    "malformed yaml",
			path:    writeImagesFile(t, "images: [unclosed"),
			wantErr: "failed parsing YAML configuration",
		},
		{
			name:    "no images",
			path:    writeImagesFile(t, "images: []"),
			wantErr: "no images defined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(tt.path)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load() error = %v; want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestValidate verifies every image needs a source, a destination and a tag,
// and that all problems are reported at once.
func TestValidate(t *testing.T) {
	cfg := Config{Images: []ImageEntry{
		{Name: "ok", Source: "src", Destination: "dst", Tag: "1"},
		{Name: "no-tag", Source: "src", Destination: "dst", Tag: "  "},
		{Source: "src"},
	}}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() = nil; want an error")
	}

	for _, want := range []string{
		"image 2 (no-tag) is missing tag",
		"image 3 is missing destination, tag",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Validate() error = %q; want containing %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "image 1") {
		t.Errorf("Validate() error = %q; the valid image must not be reported", err)
	}
}
