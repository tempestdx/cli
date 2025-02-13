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
		After: nil,
	})
	if err != nil {
		return fmt.Errorf("list recipes: %w", err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	cmd.Println("Recipes:")
	for _, edge := range res.JSON200.Edges {
		recipe := edge.Node
		cmd.Printf("- ID: %s\n", recipe.Id)
		cmd.Printf("  Name: %s\n", recipe.Name)
		cmd.Printf("  Type: %s\n", recipe.Type)
		if recipe.PublishedAt != nil {
			cmd.Printf("  Published: %s\n", recipe.PublishedAt.Format(time.RFC3339))
		}
		cmd.Println()
	}

	if res.JSON200.PageInfo.HasNextPage {
		cmd.Println("More recipes available. Use pagination to see more.")
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
	cmd.Printf("Recipe Details:\n")
	cmd.Printf("ID: %s\n", recipe.Id)
	cmd.Printf("Name: %s\n", recipe.Name)
	cmd.Printf("Type: %s\n", recipe.Type)
	cmd.Printf("Team ID: %s\n", recipe.TeamId)
	cmd.Printf("Created: %s\n", recipe.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", recipe.UpdatedAt.Format(time.RFC3339))
	cmd.Printf("Public: %v\n", recipe.Public)
	cmd.Printf("Published: %v\n", recipe.Published)
	if recipe.PublishedAt != nil {
		cmd.Printf("Published At: %s\n", recipe.PublishedAt.Format(time.RFC3339))
	}

	return nil
}
