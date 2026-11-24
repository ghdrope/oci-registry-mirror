package mirror

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"go.uber.org/zap"
)

// execCommand allows overriding the execution call during unit tests.
var execCommand = exec.CommandContext

// retryTimes is how many times skopeo retries a failed registry request.
const retryTimes = "3"

// notFoundMarkers are the (lowercase) skopeo error messages that mean the
// image or tag is not in the registry. Registries word it differently:
// "manifest unknown" (Docker distribution, GHCR, GitLab, ECR), "name unknown"
// (missing repository) or "not found" (Harbor, Artifactory).
var notFoundMarkers = []string{"manifest unknown", "name unknown", "not found"}

// ImageExists inspects the target registry using 'skopeo inspect' to verify if the image version exists.
//
// It returns true if the image is found and false if skopeo reports it as
// missing. Any other failure (wrong credentials, registry unreachable,
// skopeo not installed, ...) is returned as an error: treating it as
// "missing" would hide it, especially in dry-run mode.
//
// --raw only fetches the manifest, so multi-arch images are found even when
// none of their platforms matches the machine running the mirror.
func ImageExists(ctx context.Context, image string, credentials string) (bool, error) {
	cmd := execCommand(ctx, "skopeo", "inspect", "--raw", "--retry-times", retryTimes, "--creds", credentials, image)

	var stderr bytes.Buffer
	cmd.Stdout = nil
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return true, nil
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false, err
	}

	message := strings.TrimSpace(stderr.String())
	lower := strings.ToLower(message)
	for _, marker := range notFoundMarkers {
		if strings.Contains(lower, marker) {
			return false, nil
		}
	}

	if message == "" {
		return false, fmt.Errorf("skopeo inspect failed: %w", err)
	}
	return false, fmt.Errorf("skopeo inspect failed: %w: %s", err, message)
}

// CopyImage replicates an OCI image from source public registry to target private destination using 'skopeo copy'.
//
// --all copies every platform of a multi-arch image (and keeps the index),
// instead of only the platform of the machine running the mirror.
func CopyImage(ctx context.Context, source, destination, credentials string) error {
	cmd := execCommand(ctx, "skopeo", "copy", "--all", "--retry-times", retryTimes, "--dest-creds", credentials, source, destination)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// imageExists and copyImage are the skopeo calls used by Run. They are
// variables so tests can replace them.
var (
	imageExists = ImageExists
	copyImage   = CopyImage
)

// Summary counts the images handled by Run.
type Summary struct {
	Total    int // images in the configuration
	Skipped  int // already present at the destination
	Mirrored int // copied (or that would be copied, in dry-run mode)
}

// Run mirrors every image to its destination, tagged with its own tag and
// with "latest".
//
// An image whose tag already exists at the destination is skipped, so the
// explicit tag is copied last: it marks the image as complete, and an
// interrupted run copies "latest" again next time. Images tagged "latest"
// are never skipped, since that tag moves and must be refreshed.
//
// In dry-run mode the destination is still inspected (which checks the
// credentials), but nothing is copied.
func Run(ctx context.Context, logger *zap.Logger, images []config.ImageEntry, credentials string, dryRun bool) (Summary, error) {
	var summary Summary

	for _, entry := range images {
		logger.Info("Processing image",
			zap.String("name", entry.Name),
			zap.String("source", fmt.Sprintf("%s:%s", entry.Source, entry.Tag)),
		)

		summary.Total++

		if err := ctx.Err(); err != nil {
			return summary, fmt.Errorf("mirror execution cancelled: %w", err)
		}

		if entry.Tag != "latest" {
			explicitVersionImage := fmt.Sprintf("docker://%s:%s", entry.Destination, entry.Tag)

			exists, err := imageExists(ctx, explicitVersionImage, credentials)
			if err != nil {
				return summary, fmt.Errorf("failed checking image existence for %s: %w", explicitVersionImage, err)
			}

			if exists {
				logger.Info("Explicit version already exists, skipping",
					zap.String("destination", entry.Destination),
					zap.String("tag", entry.Tag),
				)
				summary.Skipped++
				continue
			}
		}

		sourceImage := fmt.Sprintf("docker://%s:%s", entry.Source, entry.Tag)

		for _, destinationTag := range destinationTags(entry.Tag) {
			if err := ctx.Err(); err != nil {
				return summary, fmt.Errorf("mirror execution cancelled: %w", err)
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

			if err := copyImage(ctx, sourceImage, destinationImage, credentials); err != nil {
				return summary, fmt.Errorf("skopeo copy failed for %s:%s: %w", entry.Destination, destinationTag, err)
			}

			logger.Info("Successfully mirrored image",
				zap.String("destination", entry.Destination),
				zap.String("tag", destinationTag),
			)
		}

		summary.Mirrored++
	}

	return summary, nil
}

// destinationTags returns the tags an image is copied to: "latest" first and
// the explicit tag last (see Run).
func destinationTags(tag string) []string {
	if tag == "latest" {
		return []string{"latest"}
	}
	return []string{"latest", tag}
}
