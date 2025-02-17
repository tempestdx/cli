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

	var allResources []appapi.Resource
	var nextToken string

	for {
		res, err := tempestClient.ResourceCollectionWithResponse(context.TODO(), appapi.ResourceCollectionJSONRequestBody{
			Next:        nextToken,
		})
		if err != nil {
			return fmt.Errorf("list resources: %w", err)
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

		allResources = append(allResources, res.JSON200.Resources...)

		if res.JSON200.Next == "" {
			break
		}
		nextToken = res.JSON200.Next
	}

	resources := allResources
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

	totalCount := len(allResources)
	if headFlag > 0 || tailFlag > 0 {
		cmd.Printf("Showing %d of %d resources\n", len(resources), totalCount)
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

	resource := res.JSON200
	name := "-"
	if resource.Name != nil {
		name = *resource.Name
	}

	cmd.Printf("Name:\t%s\n", name)
	cmd.Printf("ID:\t%s\n", resource.Id)
	externalID := "-"
	if resource.ExternalId != "" {
		externalID = resource.ExternalId
	}
	cmd.Printf("External ID:\t%s\n", externalID)

	externalURL := "-"
	if resource.ExternalUrl != nil {
		externalURL = *resource.ExternalUrl
	}
	cmd.Printf("External URL:\t%s\n", externalURL)
	cmd.Println()

	cmd.Println("Metadata:")
	cmd.Printf("  Type:\t%s\n", resource.Type)
	orgID := "-"
	if resource.OrganizationId != nil {
		orgID = *resource.OrganizationId
	}
	cmd.Printf("  Organization ID:\t%s\n", orgID)

	createdBy := "-"
	if resource.CreatedBy != nil {
		createdBy = *resource.CreatedBy
	}
	cmd.Printf("  Created By:\t%s\n", createdBy)

	createdAt := "-"
	if resource.CreatedAt != nil {
		createdAt = resource.CreatedAt.Format(time.RFC3339)
	}
	cmd.Printf("  Creation Timestamp:\t%s\n", createdAt)

	updatedAt := "-"
	if resource.UpdatedAt != nil {
		updatedAt = resource.UpdatedAt.Format(time.RFC3339)
	}
	cmd.Printf("  Last Updated:\t%s\n", updatedAt)

	syncedAt := "-"
	if resource.SyncedAt != nil {
		syncedAt = resource.SyncedAt.Format(time.RFC3339)
	}
	cmd.Printf("  Last Synced:\t%s\n", syncedAt)
	cmd.Println()

	cmd.Println("Properties:")
	if resource.Properties != nil && len(*resource.Properties) > 0 {
		for key, value := range *resource.Properties {
			cmd.Printf("  %s:\t%v\n", key, value)
		}
	} else {
		cmd.Printf("  -\n")
	}
	cmd.Println()

	cmd.Println("Status:")
	status := "-"
	if resource.Status != nil {
		status = *resource.Status
	}
	cmd.Printf("  Status:\t%s\n", status)

	return nil
}
