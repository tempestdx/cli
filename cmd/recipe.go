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

	recipeListCmd.Flags().IntVar(&headFlag, "head", 0, "Show first n recipes")
	recipeListCmd.Flags().IntVar(&tailFlag, "tail", 0, "Show last n recipes")
	recipeListCmd.MarkFlagsMutuallyExclusive("head", "tail")
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

	res, err := tempestClient.RecipeCollectionWithResponse(context.TODO(), appapi.RecipeCollectionJSONRequestBody{
		Next: nil,
	})
	if err != nil {
		return fmt.Errorf("list recipes: %w", err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	recipes := res.JSON200.Recipes
	if headFlag > 0 && headFlag < len(recipes) {
		recipes = recipes[:headFlag]
	} else if tailFlag > 0 && tailFlag < len(recipes) {
		recipes = recipes[len(recipes)-tailFlag:]
	}

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

	totalCount := len(res.JSON200.Recipes)
	if headFlag > 0 || tailFlag > 0 {
		cmd.Printf("Showing %d of %d recipes\n", len(recipes), totalCount)
	}

	if res.JSON200.Next != "" {
		cmd.Printf("More recipes available. Use --after %s to see more.\n", res.JSON200.Next)
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
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	recipe := res.JSON200

	// Main Information (required fields from the schema)
	cmd.Printf("Name:\t%s\n", recipe.Name)
	cmd.Printf("ID:\t%s\n", recipe.Id)
	cmd.Printf("Type:\t%s\n", recipe.Type)
	cmd.Println()

	// Metadata
	cmd.Println("Metadata:")
	if recipe.TeamId != nil {
		cmd.Printf("  Team ID:\t%s\n", *recipe.TeamId)
	}
	if recipe.CreatedAt != nil {
		cmd.Printf("  Creation Timestamp:\t%s\n", recipe.CreatedAt.Format(time.RFC3339))
	}
	if recipe.UpdatedAt != nil {
		cmd.Printf("  Last Updated:\t%s\n", recipe.UpdatedAt.Format(time.RFC3339))
	}
	cmd.Println()

	// Status
	cmd.Println("Status:")
	if recipe.Public != nil {
		cmd.Printf("  Public:\t%v\n", *recipe.Public)
	}
	if recipe.Published != nil {
		cmd.Printf("  Published:\t%v\n", *recipe.Published)
	}
	if recipe.PublishedAt != nil {
		cmd.Printf("  Published At:\t%s\n", recipe.PublishedAt.Format(time.RFC3339))
	}

	return nil
}
