package main

import (
	"context"

	"github.com/ghdrope/oci-registry-mirror/internal/config"
	"github.com/spf13/cobra"
)

// configPath is the images file used by the mirror and scan commands.
var configPath string

// rootCmd defines the base CLI endpoint.
//
// It exists to:
//   - provide CLI structure
//   - enable context propagation across Cobra command tree
//   - display help when invoked without arguments
var rootCmd = &cobra.Command{
	Use:   "oci-registry-mirror",
	Short: "OCI Registry Mirror CLI",
	Long:  "OCI Registry Mirror automates copying OCI images from public registries to private registries using skopeo",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior when no subcommand is provided
		_ = cmd.Help()
	},
}

// Execute runs the CLI command tree using the provided context.
//
// The context is propagated to all subcommands.
func Execute(ctx context.Context) error {
	return rootCmd.ExecuteContext(ctx)
}

func init() {
	rootCmd.PersistentFlags().StringVar(&configPath, "config", config.DefaultPath, "Path to the images file")

	// Register runtime subcommands
	rootCmd.AddCommand(newMirrorCommand())
	rootCmd.AddCommand(newScanCommand())
	rootCmd.AddCommand(newVersionCommand())
}
