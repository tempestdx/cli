package cmd

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/tempestdx/cli/internal/secret"
	"github.com/tempestdx/cli/internal/version"
)

var (
	apiEndpoint string
	cfgFile     string
	tokenStore  secret.TokenStore

	rootCmd = &cobra.Command{
		Use:     "tempest [command] [flags]",
		Short:   "Tempest is a CLI tool to interact with the Tempest API and SDK",
		Version: version.Version,
	}

	// Add a command to generate the markdown documentation.
	docCmd = &cobra.Command{
		Use:    "gendocs",
		Short:  "Generate markdown documentation for the CLI tool",
		Hidden: true, // This will hide the command from the help
		Run: func(cmd *cobra.Command, args []string) {
			directory := "./tempest-cli-docs"
			err := os.MkdirAll(directory, 0o755)
			if err != nil {
				log.Fatalln(err)
			}
			rootCmd.DisableAutoGenTag = true
			err = doc.GenMarkdownTree(rootCmd, directory)
			if err != nil {
				log.Fatalln(err)
			}
		},
	}
)

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalln(err)
	}
}

func init() {
	rootCmd.AddCommand(docCmd)
	rootCmd.PersistentFlags().StringVar(&apiEndpoint, "api-endpoint", TempestProdAPI, "The Tempest API endpoint to connect to.")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "full path to the config file (default is $WORKDIR/tempest.yaml)")

	tokenStore = &secret.Keyring{}
}
