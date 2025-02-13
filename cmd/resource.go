package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/secret"
	appapi "github.com/tempestdx/openapi/app"
)

var resourceCmd = &cobra.Command{
	Use:   "resource",
	Short: "Manage resources",
	Long:  `List and get resources from your Tempest App`,
}

var resourceListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all resources",
	Args:  cobra.NoArgs,
	RunE:  listResources,
}

var resourceGetCmd = &cobra.Command{
	Use:   "get <resource_id>",
	Short: "Get a specific resource",
	Args:  cobra.ExactArgs(1),
	RunE:  getResource,
}

func init() {
	rootCmd.AddCommand(resourceCmd)
	resourceCmd.AddCommand(resourceListCmd)
	resourceCmd.AddCommand(resourceGetCmd)
}

func listResources(cmd *cobra.Command, args []string) error {
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

	res, err := tempestClient.ResourceCollectionWithResponse(context.TODO(), appapi.ResourceCollectionJSONRequestBody{
		Next:        "",
		RequestType: "list_resources",
	})
	if err != nil {
		return fmt.Errorf("list resources: %w", err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	cmd.Println("Resources:")
	for _, resource := range res.JSON200.Resources {
		cmd.Printf("- ID: %s\n", resource.Id)
		cmd.Printf("  Name: %s\n", *resource.Name)
		cmd.Printf("  Type: %s\n", *resource.Type)
		if resource.OrganizationId != nil {
			cmd.Printf("  Organization ID: %s\n", *resource.OrganizationId)
		}
		cmd.Println()
	}

	if res.JSON200.Next != "" {
		cmd.Println("More resources available. Use pagination to see more.")
	}

	return nil
}

func getResource(cmd *cobra.Command, args []string) error {
	resourceID := args[0]
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

	body := appapi.GetResourcesJSONRequestBody{Id: resourceID}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	res, err := tempestClient.GetResourcesWithBodyWithResponse(context.TODO(), "POST", bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("get resource: %w", err)
	}

	if res.JSON200 == nil {
		return fmt.Errorf("unexpected response: %s", res.Status())
	}

	resource := res.JSON200
	cmd.Printf("Resource Details:\n")
	cmd.Printf("ID: %s\n", resource.Id)
	cmd.Printf("Name: %s\n", *resource.Name)
	if resource.Type != nil {
		cmd.Printf("Type: %s\n", *resource.Type)
	}
	if resource.OrganizationId != nil {
		cmd.Printf("Organization ID: %s\n", *resource.OrganizationId)
	}
	cmd.Printf("Created: %s\n", resource.CreatedAt.Format(time.RFC3339))
	cmd.Printf("Updated: %s\n", resource.UpdatedAt.Format(time.RFC3339))

	return nil
}
