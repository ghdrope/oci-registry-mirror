package config

import (
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"
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
			name: "valid configuration with multiple tags",
			yamlData: `
images:
  - source: docker.io/library/ubuntu
    destination: harbor.com/originals/ubuntu
    tag:
      - "20.04"
      - "22.04"
`,
			expectError: false,
			expected: Config{
				Images: []ImageEntry{
					{
						Source:      "docker.io/library/ubuntu",
						Destination: "harbor.com/originals/ubuntu",
						Tag:         []string{"20.04", "22.04"},
					},
				},
			},
		},
		{
			name: "empty image list is allowed by parser",
			yamlData: `
images: []
`,
			expectError: false,
			expected:    Config{Images: []ImageEntry{}},
		},
		{
			name: "malformed yaml structure triggers error",
			yamlData: `
images:
  - source: docker.io/library/ubuntu
    destination: harbor.com/originals/ubuntu
    tag: [invalid sequence nesting
`,
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
