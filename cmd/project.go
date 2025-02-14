package cmd

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/charmbracelet/glamour"
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

	projectListCmd.Flags().IntVar(&headFlag, "head", 0, "Show first n projects")
	projectListCmd.Flags().IntVar(&tailFlag, "tail", 0, "Show last n projects")
	projectListCmd.MarkFlagsMutuallyExclusive("head", "tail")
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
		Next: nil,
	})
	if err != nil {
		return fmt.Errorf("list projects: %w", err)
	}

	if res.JSON200 == nil {
		if res.JSON400 != nil {
			return fmt.Errorf("bad request: %s", res.JSON400.Error)
		}
		if res.JSON500 != nil {
			return fmt.Errorf("server error: %s", res.JSON500.Error)
		}
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	projects := res.JSON200.Projects
	if headFlag > 0 && headFlag < len(projects) {
		projects = projects[:headFlag]
	} else if tailFlag > 0 && tailFlag < len(projects) {
		projects = projects[len(projects)-tailFlag:]
	}

	table := "| ID | Name | Type | From Recipe | Organization ID | Team ID |\n"
	table += "|----|------|------|-------------|-----------------|----------|\n"

	for _, project := range projects {
		fromRecipe := " "
		if project.FromRecipe != nil {
			fromRecipe = *project.FromRecipe
		}
		teamID := " "
		if project.TeamId != nil {
			teamID = *project.TeamId
		}
		table += fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
			project.Id,
			project.Name,
			project.Type,
			fromRecipe,
			project.OrganizationId,
			teamID,
		)
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(120),
	)
	if err != nil {
		return fmt.Errorf("create renderer: %w", err)
	}

	out, err := renderer.Render(table)
	if err != nil {
		return fmt.Errorf("render table: %w", err)
	}
	cmd.Print(out)

	totalCount := len(res.JSON200.Projects)
	if headFlag > 0 || tailFlag > 0 {
		cmd.Printf("Showing %d of %d projects\n", len(projects), totalCount)
	}

	if res.JSON200.Next != "" {
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
		if res.JSON400 != nil {
			return fmt.Errorf("bad request: %s", res.JSON400.Error)
		}
		if res.JSON404 != nil {
			return fmt.Errorf("not found: %s", res.JSON404.Error)
		}
		if res.JSON500 != nil {
			return fmt.Errorf("server error: %s", res.JSON500.Error)
		}
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	project := res.JSON200
	cmd.Printf("Name:\t%s\n", project.Name)
	cmd.Printf("ID:\t%s\n", project.Id)
	cmd.Println()

	cmd.Println("Metadata:")
	cmd.Printf("  Type:\t%s\n", project.Type)
	cmd.Printf("  Organization ID:\t%s\n", project.OrganizationId)
	if project.TeamId != nil {
		cmd.Printf("  Team ID:\t%s\n", *project.TeamId)
	}
	if project.CreatedAt != nil {
		cmd.Printf("  Creation Timestamp:\t%s\n", project.CreatedAt.Format(time.RFC3339))
	}
	if project.UpdatedAt != nil {
		cmd.Printf("  Last Updated:\t%s\n", project.UpdatedAt.Format(time.RFC3339))
	}
	cmd.Println()

	cmd.Println("Status:")
	if project.Published != nil {
		cmd.Printf("  Published:\t%v\n", *project.Published)
	}
	if project.FromRecipe != nil {
		cmd.Printf("  From Recipe:\t%s\n", *project.FromRecipe)
	}

	return nil
}
