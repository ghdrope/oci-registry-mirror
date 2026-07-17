package main

import "github.com/spf13/cobra"

// newScanCommand defines the runtime entrypoint for initiating
// the image vulnerability inspection procedure.
//
// This command is responsible for:
//   - wrapping the runScan execution inside Cobra command structure
func newScanCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan listed images for HIGH and CRITICAL vulnerabilities",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runScan(cmd.Context())
		},
	}

	return cmd
}
