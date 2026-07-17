package config

// Config represents the structural definition of the images.yaml file.
type Config struct {
	Images []ImageEntry `yaml:"images"`
}

// ImageEntry defines the source, and destination container registries, and tag to be mirrored.
type ImageEntry struct {
	Name             string `yaml:"name"`
	Source           string `yaml:"source"`
	Destination      string `yaml:"destination"`
	Tag              string `yaml:"tag"`
	IgnoreSeverities bool   `yaml:"ignore-severities"`
}
