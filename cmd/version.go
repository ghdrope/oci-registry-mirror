package main

import (
	"fmt"

	version "github.com/ghdrope/go-version"
	"github.com/spf13/cobra"
)

// newVersionCommand creates the version subcommand.
func newVersionCommand() *cobra.Command {
	var short bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Displays version, git commit and build date information.",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			if short {
				fmt.Println(version.Short())
				return
			}
			fmt.Println(version.String())
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "Print only the version string")

	return cmd
}
