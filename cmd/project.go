package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/secret"
	appapi "github.com/tempestdx/openapi/app"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
	Long:  `List and get projects from your Tempest App`,
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	Args:  cobra.NoArgs,
	RunE:  listProjects,
}

var projectGetCmd = &cobra.Command{
	Use:   "get <project_id>",
	Short: "Get a specific project",
	Args:  cobra.ExactArgs(1),
	RunE:  getProject,
}

func init() {
	rootCmd.AddCommand(projectCmd)
	projectCmd.AddCommand(projectListCmd)
	projectCmd.AddCommand(projectGetCmd)
}

func listProjects(cmd *cobra.Command, args []string) error {
	token := loadTempestToken(cmd)

	tempestClient, err := appapi.NewClientWithResponses(
		apiEndpoint,
		appapi.WithHTTPClient(&http.Client{
			Timeout:   10 * time.Second,
			Transport: secret.NewTransportWithToken(token),
		}),
	)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	res, err := tempestClient.ProjectCollectionWithResponse(context.TODO(), appapi.ProjectCollectionJSONRequestBody{
		After: nil,
	})
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	cmd.Println("Projects:")
	for _, edge := range res.JSON200.Edges {
		project := edge.Node
		cmd.Printf("- ID: %s\n", project.Id)
		cmd.Printf("  Name: %s\n", project.Name)
		cmd.Printf("  Type: %s\n", project.Type)
		cmd.Printf("  From Recipe: %s\n", *project.FromRecipe)
		cmd.Printf("  Organization ID: %s\n", project.OrganizationId)
		cmd.Printf("  Team ID: %s\n", project.TeamId)
		cmd.Println()
	}

	if res.JSON200.PageInfo.HasNextPage {
		cmd.Println("More projects available. Use pagination to see more.")
	}

	return nil
}

func getProject(cmd *cobra.Command, args []string) error {
	projectID := args[0]
	token := loadTempestToken(cmd)

	tempestClient, err := appapi.NewClientWithResponses(
		apiEndpoint,
		appapi.WithHTTPClient(&http.Client{
			Timeout:   10 * time.Second,
			Transport: secret.NewTransportWithToken(token),
		}),
	)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	res, err := tempestClient.GetProjectWithResponse(context.TODO(), appapi.GetProjectJSONRequestBody{
		Id: projectID,
	})
	if err != nil {
		return fmt.Errorf("get project: %w", err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	project := res.JSON200
	cmd.Printf("Project Details:\n")
	cmd.Printf("ID: %s\n", project.Id)
	cmd.Printf("Name: %s\n", project.Name)
	cmd.Printf("Type: %s\n", project.Type)
	cmd.Printf("Organization ID: %s\n", project.OrganizationId)
	cmd.Printf("Team ID: %s\n", project.TeamId)
	cmd.Printf("Created: %s\n", project.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", project.UpdatedAt.Format(time.RFC3339))
	cmd.Printf("From Recipe: %s\n", *project.FromRecipe)
	cmd.Printf("Published: %v\n", *project.Published)

	return nil
}
