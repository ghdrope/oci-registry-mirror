package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"github.com/ghdrope/oci-registry-mirror/internal/mirror"
	"github.com/ghdrope/oci-registry-mirror/pkg/env"
	"go.uber.org/zap"
	"go.yaml.in/yaml/v2"
)

const imagesFilePath = "images.yaml"

// runMirror bootstraps and runs the image replication logic
//
// It wires together:
//   - environment variables retrieval (fails fast on missing variables)
//   - configuration loading from local yaml
//   - execution of replication loops via skopeo commands
//
// The process runs sequentially and aborts immediately on cancel signals or execution errors.
func runMirror(cmdCtx context.Context, dryRun bool) (err error) {
	logger := zap.L().With(zap.String("service", "mirror"))
	if dryRun {
		logger.Info("Starting OCI registry image mirror process [DRY-RUN MODE Active]")
	} else {
		logger.Info("Starting OCI registry image mirror process")
	}

	// ---------------------------
	// CONFIGURATION
	// ---------------------------
	// Enforce strict configuration checking. This will panic and crash the
	// process cleanly if environment variables are missing

	registryUsername := env.Must("REGISTRY_USERNAME")
	registryPassword := env.Must("REGISTRY_PASSWORD")

	credentials := fmt.Sprintf("%s:%s", registryUsername, registryPassword)
	logger.Info("configuration loaded successfully")

	// ---------------------------
	// CONFIGURATION
	// ---------------------------
	if _, err := os.Stat(imagesFilePath); os.IsNotExist(err) {
		return fmt.Errorf("image configuration file '%s' does not exist", imagesFilePath)
	}

	fileBytes, err := os.ReadFile(imagesFilePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", imagesFilePath, err)
	}

	var config config.Config
	if err := yaml.Unmarshal(fileBytes, &config); err != nil {
		return fmt.Errorf("failed parsing YAML configuration: %w", err)
	}

	if len(config.Images) == 0 {
		return fmt.Errorf("no images defined in %s", imagesFilePath)
	}

	// ---------------------------
	// MIRROR PROCESS LOOP
	// ---------------------------
	var total, skipped, mirrored int

	for _, entry := range config.Images {
		for _, tag := range entry.Tag {
			// Interrupt process safely if context gets cancelled
			if err := cmdCtx.Err(); err != nil {
				return fmt.Errorf("mirror execution cancelled: %w", err)
			}

			total++
			sourceImage := fmt.Sprintf("docker://%s:%s", entry.Source, tag)
			destinationImage := fmt.Sprintf("docker://%s:%s", entry.Destination, tag)

			logger.Info("Checking destination image", zap.String("image", destinationImage))

			exists, err := mirror.ImageExists(cmdCtx, destinationImage, credentials)
			if err != nil {
				return fmt.Errorf("failed checking image existence: %w", err)
			}

			if exists {
				logger.Info("Image already exists, skipping",
					zap.String("destination", entry.Destination),
					zap.String("tag", tag),
				)
				skipped++
				continue
			}

			if dryRun {
				logger.Info("[DRY-RUN] Image would be mirrored",
					zap.String("from", sourceImage),
					zap.String("to", destinationImage),
				)
				mirrored++
				continue
			}

			logger.Info("Mirroring image",
				zap.String("from", sourceImage),
				zap.String("to", destinationImage),
			)

			if err := mirror.CopyImage(cmdCtx, sourceImage, destinationImage, credentials); err != nil {
				return fmt.Errorf("skopeo copy failed for %s:%s: %w", entry.Destination, tag, err)
			}

			logger.Info("Successfully mirrored image",
				zap.String("destination", entry.Destination),
				zap.String("tag", tag),
			)
			mirrored++
		}
	}

	logger.Info(strings.Repeat("=", 70))
	if dryRun {
		logger.Info("Mirror dry-run process completed")
	} else {
		logger.Info("Mirror process completed")
	}
	logger.Info("Total images checked", zap.Int("total", total))
	logger.Info("Already existing", zap.Int("skipped", skipped))
	mirrorLabel := "Newly mirrored"
	if dryRun {
		mirrorLabel = "To be mirrored (simulated)"
	}
	logger.Info(mirrorLabel, zap.Int("mirrored", mirrored))
	logger.Info(strings.Repeat("=", 70))

	return nil
}
