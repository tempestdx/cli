/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/config"
	"github.com/tempestdx/cli/internal/runner"
	appv1 "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1"
)

// describeCmd represents the describe command.
var describeCmd = &cobra.Command{
	Use:   "describe <app_id:app_version>",
	Short: "View capabilities of your Tempest App",
	Long:  `View the resources supported and operations supported by your Tempest Private App`,
	Args:  cobra.ExactArgs(1),
	RunE:  describeApp,
}

func init() {
	appCmd.AddCommand(describeCmd)
}

func splitAppVersion(appIDVersion string) (string, string, error) {
	parts := strings.Split(appIDVersion, ":")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid app ID and version format")
	}

	return parts[0], parts[1], nil
}

func describeApp(cmd *cobra.Command, args []string) error {
	id, version, err := splitAppVersion(args[0])
	if err != nil {
		return err
	}

	cfg, cfgDir, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	appVersion := cfg.LookupAppByVersion(id, version)
	if appVersion == nil {
		return fmt.Errorf("app %s:%s not found", id, version)
	}

	if !appPreserveBuildDir {
		err := generateBuildDir(cfg, cfgDir, id, version)
		if err != nil {
			return fmt.Errorf("generate build dir: %w", err)
		}
	}

	runners, cancel, err := runner.StartApps(context.TODO(), cfg, cfgDir)
	if err != nil {
		return fmt.Errorf("start local app: %w", err)
	}
	defer cancel()

	var runner runner.Runner
	for _, r := range runners {
		if r.AppID == id && r.Version == version {
			runner = r
			break
		}
	}

	res, err := runner.Client.Describe(context.TODO(), connect.NewRequest(&appv1.DescribeRequest{}))
	if err != nil {
		return fmt.Errorf("reach private app: %w", err)
	}

	cmd.Println(`Tempest App Description
-----------------------`)

	cmd.Println(formatDescribeResponse(res.Msg, id, appVersion))

	return nil
}

func formatDescribeResponse(res *appv1.DescribeResponse, appID string, version *config.AppVersion) string {
	s := strings.Builder{}

	absolutePath, err := filepath.Abs(version.Path)
	if err != nil {
		absolutePath = version.Path
	}

	s.WriteString(fmt.Sprintf(`
Describing app: %s:%s
Location: %s

`, appID, version.Version, absolutePath))

	for _, r := range res.GetResourceDefinitions() {
		s.WriteString(fmt.Sprintf("Resource Type: %s\n", r.DisplayName))

		if len(r.Links) > 0 {
			s.WriteString("\nLinks:\n")
			for _, link := range r.Links {
				s.WriteString(fmt.Sprintf("- %s: %s (%s)\n", link.Title, link.Url, link.Type.String()))
			}
		}

		s.WriteString("\nOperations Supported:\n")
		s.WriteString("- ")
		s.WriteString(boolToCheckmark(r.ReadSupported))
		s.WriteString(" Read\n")
		s.WriteString("- ")
		s.WriteString(boolToCheckmark(r.ListSupported))
		s.WriteString(" List\n")
		s.WriteString("- ")
		s.WriteString(boolToCheckmark(r.CreateSupported))
		s.WriteString(" Create\n")
		s.WriteString("- ")
		s.WriteString(boolToCheckmark(r.UpdateSupported))
		s.WriteString(" Update\n")
		s.WriteString("- ")
		s.WriteString(boolToCheckmark(r.DeleteSupported))
		s.WriteString(" Delete\n")

		s.WriteString(fmt.Sprintf("\nHealth Check Supported: %s\n", boolToCheckmark(r.HealthcheckSupported)))

		if r.InstructionsMarkdown != "" {
			s.WriteString("\nUsage Instructions and Details:\n")
			s.WriteString(r.InstructionsMarkdown)
		}
	}

	return s.String()
}

func boolToCheckmark(b bool) string {
	if b {
		return "✅"
	}
	return "❌"
}
