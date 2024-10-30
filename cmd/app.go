package cmd

import (
	"github.com/spf13/cobra"
)

var (
	appPreserveBuildDir bool

	appCmd = &cobra.Command{
		Use:   "app [command] [flags]",
		Short: "Manage Tempest Apps",
	}
)

func init() {
	rootCmd.AddCommand(appCmd)

	appCmd.PersistentFlags().BoolVar(&appPreserveBuildDir, "preserve-build-dir", false, "Preserve the existing build directory. Useful for debugging")
}
