package cmd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
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

	projectListCmd.Flags().IntVar(&limitFlag, "limit", 0, "Limit the number of projects shown")
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

	var allProjects []appapi.Project
	var nextToken *string
	pageCount := 0

	for {
		pageCount++
		res, err := tempestClient.PostProjectsListWithResponse(context.TODO(), appapi.PostProjectsListJSONRequestBody{
			Next: nextToken,
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

		allProjects = append(allProjects, res.JSON200.Projects...)

		if res.JSON200.Next == "" {
			break
		}
		nextToken = &res.JSON200.Next
	}

	if limitFlag > 0 && len(allProjects) > limitFlag {
		allProjects = allProjects[:limitFlag]
	}

	table := "| ID | Name | Type | From Recipe | Organization ID | Team ID |\n"
	table += "|----|------|------|-------------|-----------------|----------|\n"

	for _, project := range allProjects {
		var fromRecipe string
		if project.FromRecipe != nil {
			fromRecipe = *project.FromRecipe
		}
		var teamID string
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

	totalFetched := len(allProjects)
	if limitFlag > 0 {
		cmd.Printf("Showing %d/%d projects\n", len(allProjects), totalFetched)
	} else {
		cmd.Printf("Showing %d projects from %d pages\n", len(allProjects), pageCount)
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

	res, err := tempestClient.PostProjectsGetWithResponse(context.TODO(), appapi.PostProjectsGetJSONRequestBody{
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

	// Main Information
	mainInfo := map[string]string{
		"Name": project.Name,
		"ID":   project.Id,
	}

	// Extract and sort keys
	mainInfoKeys := make([]string, 0, len(mainInfo))
	for key := range mainInfo {
		mainInfoKeys = append(mainInfoKeys, key)
	}
	sort.Strings(mainInfoKeys)

	// Calculate the maximum key length for main information
	maxMainInfoKeyLength := 0
	for _, key := range mainInfoKeys {
		if len(key) > maxMainInfoKeyLength {
			maxMainInfoKeyLength = len(key)
		}
	}

	// Print each main information with aligned keys
	for _, key := range mainInfoKeys {
		value := mainInfo[key]
		cmd.Printf("%-*s : %s\n", maxMainInfoKeyLength, key, value)
	}
	cmd.Println()

	// Metadata
	cmd.Println("Metadata:")
	teamID := "-"
	if project.TeamId != nil {
		teamID = *project.TeamId
	}
	createdAt := "-"
	if project.CreatedAt != nil {
		createdAt = project.CreatedAt.Format(time.RFC3339)
	}
	updatedAt := "-"
	if project.UpdatedAt != nil {
		updatedAt = project.UpdatedAt.Format(time.RFC3339)
	}

	metadata := map[string]string{
		"Type":               project.Type,
		"Organization ID":    project.OrganizationId,
		"Team ID":            teamID,
		"Creation Timestamp": createdAt,
		"Last Updated":       updatedAt,
	}

	// Extract and sort keys
	metadataKeys := make([]string, 0, len(metadata))
	for key := range metadata {
		metadataKeys = append(metadataKeys, key)
	}
	sort.Strings(metadataKeys)

	// Calculate the maximum key length for metadata
	maxMetadataKeyLength := 0
	for _, key := range metadataKeys {
		if len(key) > maxMetadataKeyLength {
			maxMetadataKeyLength = len(key)
		}
	}

	// Print each metadata with aligned keys
	for _, key := range metadataKeys {
		value := metadata[key]
		cmd.Printf("  %-*s : %s\n", maxMetadataKeyLength, key, value)
	}
	cmd.Println()

	// Status
	cmd.Println("Status:")
	published := "-"
	if project.Published != nil {
		published = fmt.Sprintf("%v", *project.Published)
	}
	fromRecipe := "-"
	if project.FromRecipe != nil {
		fromRecipe = *project.FromRecipe
	}

	status := map[string]string{
		"Published":   published,
		"From Recipe": fromRecipe,
	}

	// Extract and sort keys
	statusKeys := make([]string, 0, len(status))
	for key := range status {
		statusKeys = append(statusKeys, key)
	}
	sort.Strings(statusKeys)

	// Calculate the maximum key length for status
	maxStatusKeyLength := 0
	for _, key := range statusKeys {
		if len(key) > maxStatusKeyLength {
			maxStatusKeyLength = len(key)
		}
	}

	// Print each status with aligned keys
	for _, key := range statusKeys {
		value := status[key]
		cmd.Printf("  %-*s : %s\n", maxStatusKeyLength, key, value)
	}

	return nil
}
