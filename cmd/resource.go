package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/charmbracelet/glamour"
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

	resourceListCmd.Flags().IntVar(&headFlag, "head", 0, "Show first n resources")
	resourceListCmd.Flags().IntVar(&tailFlag, "tail", 0, "Show last n resources")
	resourceListCmd.MarkFlagsMutuallyExclusive("head", "tail")
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

	resources := res.JSON200.Resources
	if headFlag > 0 && headFlag < len(resources) {
		resources = resources[:headFlag]
	} else if tailFlag > 0 && tailFlag < len(resources) {
		resources = resources[len(resources)-tailFlag:]
	}

	table := "| ID | Name | Type | Organization ID |\n"
	table += "|-------|------|------|----------------|\n"

	for _, resource := range resources {
		name := " "
		if resource.Name != nil {
			name = *resource.Name
		}
		orgID := " "
		if resource.OrganizationId != nil {
			orgID = *resource.OrganizationId
		}

		table += fmt.Sprintf("| %s | %s | %s | %s |\n",
			resource.Id,
			name,
			resource.Type,
			orgID,
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

	totalCount := len(res.JSON200.Resources)
	if headFlag > 0 || tailFlag > 0 {
		cmd.Printf("Showing %d of %d resources\n", len(resources), totalCount)
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
	name := " "
	if resource.Name != nil {
		name = *resource.Name
	}

	cmd.Printf("Name:\t%s\n", name)
	cmd.Printf("ID:\t%s\n", resource.Id)
	if resource.ExternalId != "" {
		cmd.Printf("External ID:\t%s\n", resource.ExternalId)
	}
	if resource.ExternalUrl != nil {
		cmd.Printf("External URL:\t%s\n", *resource.ExternalUrl)
	}
	cmd.Println()

	cmd.Println("Metadata:")
	cmd.Printf("  Type:\t%s\n", resource.Type)
	if resource.OrganizationId != nil {
		cmd.Printf("  Organization ID:\t%s\n", *resource.OrganizationId)
	}
	if resource.CreatedBy != nil {
		cmd.Printf("  Created By:\t%s\n", *resource.CreatedBy)
	}
	if resource.CreatedAt != nil {
		cmd.Printf("  Creation Timestamp:\t%s\n", resource.CreatedAt.Format(time.RFC3339))
	}
	if resource.UpdatedAt != nil {
		cmd.Printf("  Last Updated:\t%s\n", resource.UpdatedAt.Format(time.RFC3339))
	}
	if resource.SyncedAt != nil {
		cmd.Printf("  Last Synced:\t%s\n", resource.SyncedAt.Format(time.RFC3339))
	}
	cmd.Println()

	if resource.Properties != nil {
		cmd.Println("Properties:")
		for key, value := range *resource.Properties {
			cmd.Printf("  %s:\t%v\n", key, value)
		}
		cmd.Println()
	}

	if resource.Status != nil {
		cmd.Println("Status:")
		cmd.Printf("  Status:\t%s\n", *resource.Status)
	}

	return nil
}
