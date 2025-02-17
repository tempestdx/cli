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
		res, err := tempestClient.RecipeCollectionWithResponse(context.TODO(), appapi.RecipeCollectionJSONRequestBody{
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

		if limitFlag > 0 && len(allRecipes) >= limitFlag {
			allRecipes = allRecipes[:limitFlag]
			break
		}

		if res.JSON200.Next == "" {
			break
		}
		nextToken = &res.JSON200.Next
	}

	recipes := allRecipes

	table := "| ID | Name | Type | Team ID | Public | Published | Published At |\n"
	table += "|-------|------|------|---------|---------|-----------|-------------|\n"

	for _, recipe := range recipes {
		teamID := " "
		if recipe.TeamId != nil {
			teamID = *recipe.TeamId
		}
		public := " "
		if recipe.Public != nil {
			public = fmt.Sprintf("%v", *recipe.Public)
		}
		published := " "
		if recipe.Published != nil {
			published = fmt.Sprintf("%v", *recipe.Published)
		}
		publishedAt := " "
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

	totalCount := len(allRecipes)
	if limitFlag > 0 {
		cmd.Printf("Showing %d of %d or more recipes\n", len(recipes), totalCount)
	}

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

	res, err := tempestClient.GetRecipeWithResponse(context.TODO(), appapi.GetRecipeJSONRequestBody{
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
	cmd.Printf("Name:\t%s\n", recipe.Name)
	cmd.Printf("ID:\t%s\n", recipe.Id)
	cmd.Printf("Type:\t%s\n", recipe.Type)
	cmd.Println()

	// Metadata
	cmd.Println("Metadata:")
	teamID := "-"
	if recipe.TeamId != nil {
		teamID = *recipe.TeamId
	}
	cmd.Printf("  Team ID:\t%s\n", teamID)

	createdAt := "-"
	if recipe.CreatedAt != nil {
		createdAt = recipe.CreatedAt.Format(time.RFC3339)
	}
	cmd.Printf("  Creation Timestamp:\t%s\n", createdAt)

	updatedAt := "-"
	if recipe.UpdatedAt != nil {
		updatedAt = recipe.UpdatedAt.Format(time.RFC3339)
	}
	cmd.Printf("  Last Updated:\t%s\n", updatedAt)
	cmd.Println()

	// Status
	cmd.Println("Status:")
	public := "-"
	if recipe.Public != nil {
		public = fmt.Sprintf("%v", *recipe.Public)
	}
	cmd.Printf("  Public:\t%s\n", public)

	published := "-"
	if recipe.Published != nil {
		published = fmt.Sprintf("%v", *recipe.Published)
	}
	cmd.Printf("  Published:\t%s\n", published)

	publishedAt := "-"
	if recipe.PublishedAt != nil {
		publishedAt = recipe.PublishedAt.Format(time.RFC3339)
	}
	cmd.Printf("  Published At:\t%s\n", publishedAt)

	return nil
}
