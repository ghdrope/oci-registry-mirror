package config

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"sigs.k8s.io/yaml"
)

// DefaultPath is the images file read when no other path is given.
const DefaultPath = "images.yaml"

// Config represents the structural definition of the images.yaml file.
type Config struct {
	Images []ImageEntry `yaml:"images"`
}

// ImageEntry defines the source, and destination container registries, and tag to be mirrored.
type ImageEntry struct {
	Name             string `yaml:"name" json:"name"`
	Source           string `yaml:"source" json:"source"`
	Destination      string `yaml:"destination" json:"destination"`
	Tag              string `yaml:"tag" json:"tag"`
	IgnoreSeverities bool   `yaml:"ignore-severities" json:"ignore-severities"`
}

// Load reads, parses and validates the images file at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("image configuration file '%s' does not exist", path)
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed parsing YAML configuration %s: %w", path, err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration %s: %w", path, err)
	}

	return &cfg, nil
}

// Validate checks that there is at least one image, and that every image
// has a source, a destination and a tag. Without them skopeo would get
// references like "docker://source:" and fail with an unclear error.
func (c *Config) Validate() error {
	if len(c.Images) == 0 {
		return errors.New("no images defined")
	}

	var problems []string
	for i, entry := range c.Images {
		var missing []string
		for _, field := range []struct{ name, value string }{
			{"source", entry.Source},
			{"destination", entry.Destination},
			{"tag", entry.Tag},
		} {
			if strings.TrimSpace(field.value) == "" {
				missing = append(missing, field.name)
			}
		}
		if len(missing) == 0 {
			continue
		}

		label := fmt.Sprintf("image %d", i+1)
		if entry.Name != "" {
			label = fmt.Sprintf("image %d (%s)", i+1, entry.Name)
		}
		problems = append(problems, fmt.Sprintf("%s is missing %s", label, strings.Join(missing, ", ")))
	}

	if len(problems) > 0 {
		return errors.New(strings.Join(problems, "; "))
	}
	return nil
}
