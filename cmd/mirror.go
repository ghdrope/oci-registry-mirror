package main

import "github.com/spf13/cobra"

// newMirrorCommand defines the runtime entrypoint for starting
// the OCI Registry Mirror execution process.
//
// This command is responsible for:
//   - wrapping the runMirror execution flow inside Cobra command structure
func newMirrorCommand() *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "mirror",
		Short: "Start OCI registry image mirror process",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMirror(cmd.Context(), dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be copied without executing actual image copies")

	return cmd
}
