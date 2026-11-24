package main

import (
	"context"
	"fmt"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"github.com/ghdrope/oci-registry-mirror/internal/mirror"
	securityscanner "github.com/ghdrope/oci-registry-mirror/internal/securityScanner"
	"github.com/ghdrope/oci-registry-mirror/pkg/env"
	"go.uber.org/zap"
)

// runMirror loads the credentials and the images file, and mirrors the
// images (see mirror.Run).
func runMirror(ctx context.Context, configPath string, dryRun bool) error {
	logger := zap.L().With(zap.String("service", "mirror"))
	if dryRun {
		logger.Info("Starting OCI registry image mirror process [DRY-RUN MODE Active]")
	} else {
		logger.Info("Starting OCI registry image mirror process")
	}

	registryUsername, err := env.Require("REGISTRY_USERNAME")
	if err != nil {
		return err
	}
	registryPassword, err := env.Require("REGISTRY_PASSWORD")
	if err != nil {
		return err
	}
	credentials := fmt.Sprintf("%s:%s", registryUsername, registryPassword)

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	logger.Info("configuration loaded successfully", zap.Int("images", len(cfg.Images)))

	summary, err := mirror.Run(ctx, logger, cfg.Images, credentials, dryRun)
	if err != nil {
		return err
	}

	// In dry-run mode "mirrored" counts the images that would be copied.
	logger.Info("Mirror process completed",
		zap.Bool("dry_run", dryRun),
		zap.Int("total", summary.Total),
		zap.Int("skipped", summary.Skipped),
		zap.Int("mirrored", summary.Mirrored),
	)

	return nil
}

// runScan loads the images file and scans the source images for HIGH and
// CRITICAL vulnerabilities (see securityscanner.Run).
func runScan(ctx context.Context, configPath string) error {
	logger := zap.L().With(zap.String("service", "security-scanner"))
	logger.Info("Starting OCI configuration targets vulnerability scanner process")

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	summary, err := securityscanner.Run(ctx, logger, cfg.Images)

	logger.Info("Vulnerability scan completed",
		zap.Int("total", summary.Total),
		zap.Int("vulnerable", summary.Vulnerable),
		zap.Int("bypassed", summary.Bypassed),
	)

	return err
}
