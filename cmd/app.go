package cmd

import (
	"github.com/spf13/cobra"
)

var appCmd = &cobra.Command{
	Use:   "app [command] [flags]",
	Short: "Manage Tempest Apps",
}

func init() {
	rootCmd.AddCommand(appCmd)
}
