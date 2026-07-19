package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"github.com/ghdrope/oci-registry-mirror/internal/mirror"
	securityscanner "github.com/ghdrope/oci-registry-mirror/internal/securityScanner"
	"github.com/ghdrope/oci-registry-mirror/pkg/env"
	"go.uber.org/zap"
	"sigs.k8s.io/yaml"
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

	var cfg config.Config
	if err := yaml.Unmarshal(fileBytes, &cfg); err != nil {
		return fmt.Errorf("failed parsing YAML configuration: %w", err)
	}

	if len(cfg.Images) == 0 {
		return fmt.Errorf("no images defined in %s", imagesFilePath)
	}

	// ---------------------------
	// MIRROR PROCESS LOOP
	// ---------------------------
	var total, skipped, mirrored int

	for _, entry := range cfg.Images {
		logger.Info(strings.Repeat("-", 30))
		logger.Info("PROCESSING IMAGE TARGET",
			zap.String("name", entry.Name),
			zap.String("source", fmt.Sprintf("%s:%s", entry.Source, entry.Tag)),
		)
		logger.Info(strings.Repeat("-", 30))

		total++

		explicitVersionImage := fmt.Sprintf("docker://%s:%s", entry.Destination, entry.Tag)
		logger.Info("Checking if explicit version already exists at destination", zap.String("image", explicitVersionImage))

		exists, err := mirror.ImageExists(cmdCtx, explicitVersionImage, credentials)
		if err != nil {
			return fmt.Errorf("failed checking image existence for %s: %w", explicitVersionImage, err)
		}

		if exists {
			logger.Info("Explicit version already exists, skipping targets completely",
				zap.String("destination", entry.Destination),
				zap.String("tag", entry.Tag),
			)
			skipped++
			continue
		}

		targetTags := []string{entry.Tag}
		if entry.Tag != "latest" {
			targetTags = append(targetTags, "latest")
		}

		sourceImage := fmt.Sprintf("docker://%s:%s", entry.Source, entry.Tag)

		for _, destinationTag := range targetTags {
			// Interrupt process safely if context gets cancelled
			if err := cmdCtx.Err(); err != nil {
				return fmt.Errorf("mirror execution cancelled: %w", err)
			}

			destinationImage := fmt.Sprintf("docker://%s:%s", entry.Destination, destinationTag)

			if dryRun {
				logger.Info("[DRY-RUN] Image would be mirrored",
					zap.String("from", sourceImage),
					zap.String("to", destinationImage),
				)
				continue
			}

			logger.Info("Mirroring image",
				zap.String("from", sourceImage),
				zap.String("to", destinationImage),
			)

			if err := mirror.CopyImage(cmdCtx, sourceImage, destinationImage, credentials); err != nil {
				return fmt.Errorf("skopeo copy failed for %s:%s: %w", entry.Destination, destinationTag, err)
			}

			logger.Info("Successfully mirrored image",
				zap.String("destination", entry.Destination),
				zap.String("tag", destinationTag),
			)
		}

		mirrored++
	}

	logger.Info(strings.Repeat("=", 30))
	if dryRun {
		logger.Info("Mirror dry-run process completed")
	} else {
		logger.Info("Mirror process completed")
	}
	logger.Info("Total images checked", zap.Int("total", total))
	logger.Info("Already existing (skipped)", zap.Int("skipped", skipped))

	mirrorLabel := "Newly mirrored"
	if dryRun {
		mirrorLabel = "To be mirrored (simulated)"
	}
	logger.Info(mirrorLabel, zap.Int("mirrored", mirrored))
	logger.Info(strings.Repeat("=", 30))

	return nil
}

// runScan parses the local images.yaml config map file and verifies security bounds.
//
// It targets:
//   - identifying vulnerabilities through integration with Trivy image scanner engine
//   - blocking pipeline continuation with structural errors if vulnerabilities are caught
//   - gracefully passing bypasses if explicit 'ignore-severities: true' keys are matched
func runScan(cmdCtx context.Context) (err error) {
	logger := zap.L().With(zap.String("service", "security-scanner"))
	logger.Info("Starting OCI configuration targets vulnerability scanner process")

	// ---------------------------
	// CONFIGURATION VALIDATION
	// ---------------------------
	if _, err := os.Stat(imagesFilePath); os.IsNotExist(err) {
		return fmt.Errorf("image configuration file '%s' does not exist", imagesFilePath)
	}

	fileBytes, err := os.ReadFile(imagesFilePath)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", imagesFilePath, err)
	}

	var cfg config.Config
	if err := yaml.Unmarshal(fileBytes, &cfg); err != nil {
		return fmt.Errorf("failed parsing YAML configuration: %w", err)
	}

	if len(cfg.Images) == 0 {
		return fmt.Errorf("no images defined in %s", imagesFilePath)
	}

	// ---------------------------
	// SCANNER PROCESS LOOP
	// ---------------------------
	var totalImages, vulnerabilitiesDetected, bypassedImages int
	var failedTargets []string

	for _, entry := range cfg.Images {
		// Interrupt process safely if context gets cancelled
		if err := cmdCtx.Err(); err != nil {
			return fmt.Errorf("scan execution cancelled: %w", err)
		}

		totalImages++
		sourceImageTarget := fmt.Sprintf("%s:%s", entry.Source, entry.Tag)

		logger.Info(strings.Repeat("-", 30))
		logger.Info("SCANNING IMAGE TARGET",
			zap.String("name", entry.Name),
			zap.String("target", sourceImageTarget),
		)
		logger.Info(strings.Repeat("-", 30))

		hasVulnerabilities, err := securityscanner.CheckVulnerabilities(cmdCtx, sourceImageTarget)
		if err != nil {
			return fmt.Errorf("vulnerability scan processing failed for %s: %w", sourceImageTarget, err)
		}

		if hasVulnerabilities {
			vulnerabilitiesDetected++

			if entry.IgnoreSeverities {
				logger.Warn("⚠️ Security vulnerabilities found but 'ignore-severities' flag is active — bypassing block risk status",
					zap.String("name", entry.Name),
					zap.String("target", sourceImageTarget),
				)
				bypassedImages++
				continue
			}

			logger.Error("❌ Target image failed security scan restrictions", zap.String("target", sourceImageTarget))
			failedTargets = append(failedTargets, sourceImageTarget)
			continue
		}

		logger.Info("✅ Target image passed vulnerability scanning completely", zap.String("target", sourceImageTarget))
	}

	logger.Info(strings.Repeat("=", 30))
	logger.Info("Vulnerability scan matrix process completed")
	logger.Info("Total target images scanned", zap.Int("total", totalImages))
	logger.Info("Vulnerable targets matched", zap.Int("vulnerable", vulnerabilitiesDetected))
	logger.Info("Bypassed exceptions (ignored)", zap.Int("bypassed", bypassedImages))
	logger.Info(strings.Repeat("=", 30))

	if len(failedTargets) > 0 {
		return fmt.Errorf(
			"🚨 security scan failed: %d target images contain un-ignored HIGH or CRITICAL vulnerabilities: [%s]",
			len(failedTargets),
			strings.Join(failedTargets, ", "),
		)
	}

	return nil
}
