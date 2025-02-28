package cmd

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/messages"
	"github.com/tempestdx/cli/internal/secret"
	appapi "github.com/tempestdx/openapi/app"
)

var recipeCmd = &cobra.Command{
	Use:   "recipe",
	Short: "Manage recipes",
	Long:  `List and get recipes from your Tempest App`,
}

var recipeListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all recipes",
	Args:  cobra.NoArgs,
	RunE:  listRecipes,
}

var recipeGetCmd = &cobra.Command{
	Use:   "get <recipe_id>",
	Short: "Get a specific recipe",
	Args:  cobra.ExactArgs(1),
	RunE:  getRecipe,
}

func init() {
	rootCmd.AddCommand(recipeCmd)
	recipeCmd.AddCommand(recipeListCmd)
	recipeCmd.AddCommand(recipeGetCmd)

	recipeListCmd.Flags().IntVar(&limitFlag, "limit", 0, "Limit the number of recipes shown")
}

func listRecipes(cmd *cobra.Command, args []string) error {
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

	var allRecipes []appapi.Recipe
	var nextToken *string

	for {
		res, err := tempestClient.PostRecipesListWithResponse(context.TODO(), appapi.PostRecipesListJSONRequestBody{
			Next: nextToken,
		})
		if err != nil {
			return fmt.Errorf("list recipes: %w", err)
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

		allRecipes = append(allRecipes, res.JSON200.Recipes...)

		if res.JSON200.Next == "" {
			break
		}
		nextToken = &res.JSON200.Next
	}

	totalFetched := len(allRecipes)

	if limitFlag > 0 && len(allRecipes) > limitFlag {
		allRecipes = allRecipes[:limitFlag]
	}

	recipes := allRecipes

	table := "| ID | Name | Type | Team ID | Public | Published | Published At |\n"
	table += "|-------|------|------|---------|---------|-----------|-------------|\n"

	for _, recipe := range recipes {
		var teamID string
		if recipe.TeamId != nil {
			teamID = *recipe.TeamId
		}
		var public string
		if recipe.Public != nil {
			public = fmt.Sprintf("%v", *recipe.Public)
		}
		var published string
		if recipe.Published != nil {
			published = fmt.Sprintf("%v", *recipe.Published)
		}
		var publishedAt string
		if recipe.PublishedAt != nil {
			publishedAt = recipe.PublishedAt.Format(time.RFC3339)
		}

		table += fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |\n",
			recipe.Id,
			recipe.Name,
			recipe.Type,
			teamID,
			public,
			published,
			publishedAt,
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

	cmd.Printf("%s\n", messages.FormatShowingSummary(len(recipes), totalFetched, "recipe"))

	return nil
}

func getRecipe(cmd *cobra.Command, args []string) error {
	recipeID := args[0]
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

	res, err := tempestClient.PostRecipesGetWithResponse(context.TODO(), appapi.PostRecipesGetJSONRequestBody{
		Id: recipeID,
	})
	if err != nil {
		return fmt.Errorf("get recipe: %w", err)
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

	recipe := res.JSON200

	// Main Information
	mainInfo := map[string]string{
		"Name": recipe.Name,
		"ID":   recipe.Id,
		"Type": recipe.Type,
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
	if recipe.TeamId != nil {
		teamID = *recipe.TeamId
	}
	createdAt := "-"
	if recipe.CreatedAt != nil {
		createdAt = recipe.CreatedAt.Format(time.RFC3339)
	}
	updatedAt := "-"
	if recipe.UpdatedAt != nil {
		updatedAt = recipe.UpdatedAt.Format(time.RFC3339)
	}

	metadata := map[string]string{
		"Team ID":            teamID,
		"Creation Timestamp": createdAt,
		"Last Updated":       updatedAt,
	}

	// Extract and sort keys
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Calculate the maximum key length for metadata
	maxMetadataKeyLength := 0
	for _, key := range keys {
		if len(key) > maxMetadataKeyLength {
			maxMetadataKeyLength = len(key)
		}
	}

	// Print each metadata with aligned keys
	for _, key := range keys {
		value := metadata[key]
		cmd.Printf("  %-*s : %s\n", maxMetadataKeyLength, key, value)
	}
	cmd.Println()

	// Status
	cmd.Println("Status:")
	public := "-"
	if recipe.Public != nil {
		public = fmt.Sprintf("%v", *recipe.Public)
	}
	published := "-"
	if recipe.Published != nil {
		published = fmt.Sprintf("%v", *recipe.Published)
	}
	publishedAt := "-"
	if recipe.PublishedAt != nil {
		publishedAt = recipe.PublishedAt.Format(time.RFC3339)
	}

	status := map[string]string{
		"Public":       public,
		"Published":    published,
		"Published At": publishedAt,
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
