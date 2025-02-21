package cmd

import (
	"bytes"
	"context"
	"encoding/json"
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

	resourceListCmd.Flags().IntVar(&limitFlag, "limit", 0, "Limit the number of resources shown")
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
	var nextToken *string
	pageCount := 0

	for {
		pageCount++
		res, err := tempestClient.PostResourcesListWithResponse(context.TODO(), appapi.PostResourcesListJSONRequestBody{
			Next: nextToken,
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
		nextToken = &res.JSON200.Next
	}

	totalFetched := len(allResources)

	if limitFlag > 0 && len(allResources) > limitFlag {
		allResources = allResources[:limitFlag]
	}

	resources := allResources

	table := "| ID | Name | Type | Organization ID |\n"
	table += "|-------|------|------|----------------|\n"

	for _, resource := range resources {
		var name string
		if resource.Name != nil {
			name = *resource.Name
		}
		var orgID string
		if resource.OrganizationId != nil {
			orgID = *resource.OrganizationId
		}

		table += fmt.Sprintf("| %s | %s | %s | %s |\n",
			*resource.Id,
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

	cmd.Print(messages.FormatShowingSummary(len(resources), totalFetched, pageCount, "resource", limitFlag > 0))

	return nil
}

type KeyValue struct {
	Key   string
	Value string
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

	body := appapi.PostResourcesGetJSONRequestBody{Id: resourceID}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	res, err := tempestClient.PostResourcesGetWithBodyWithResponse(context.TODO(), "POST", bytes.NewReader(bodyBytes))
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

	orgID := "-"
	if resource.OrganizationId != nil {
		orgID = *resource.OrganizationId
	}

	createdBy := "-"
	if resource.CreatedBy != nil {
		createdBy = *resource.CreatedBy
	}

	createdAt := "-"
	if resource.CreatedAt != nil {
		createdAt = resource.CreatedAt.Format(time.RFC3339)
	}

	updatedAt := "-"
	if resource.UpdatedAt != nil {
		updatedAt = resource.UpdatedAt.Format(time.RFC3339)
	}

	syncedAt := "-"
	if resource.SyncedAt != nil {
		syncedAt = resource.SyncedAt.Format(time.RFC3339)
	}

	externalID := "-"
	if resource.ExternalId != "" {
		externalID = resource.ExternalId
	}

	externalURL := "-"
	if resource.ExternalUrl != nil && len(*resource.ExternalUrl) > 0 {
		externalURL = *resource.ExternalUrl
	}

	// Define the fields for the initial section using KeyValue slice
	initialFields := []KeyValue{
		{"Name", name},
		{"ID", *resource.Id},
		{"External ID", externalID},
		{"External URL", externalURL},
	}

	// Calculate the maximum key length for the initial fields
	maxInitialKeyLength := 0
	for _, kv := range initialFields {
		if len(kv.Key) > maxInitialKeyLength {
			maxInitialKeyLength = len(kv.Key)
		}
	}

	// Print each initial field with aligned keys
	for _, kv := range initialFields {
		cmd.Printf("%-*s : %-30s\n", maxInitialKeyLength, kv.Key, kv.Value)
	}
	cmd.Println()

	cmd.Println("Metadata:")
	metadata := []KeyValue{
		{"Type", resource.Type},
		{"Organization ID", orgID},
		{"Created By", createdBy},
		{"Creation Timestamp", createdAt},
		{"Last Updated", updatedAt},
		{"Last Synced", syncedAt},
	}

	// Calculate the maximum key length for metadata
	maxMetadataKeyLength := 0
	for _, kv := range metadata {
		if len(kv.Key) > maxMetadataKeyLength {
			maxMetadataKeyLength = len(kv.Key)
		}
	}

	// Print each metadata with aligned keys
	for _, kv := range metadata {
		cmd.Printf("  %-*s : %-30s\n", maxMetadataKeyLength, kv.Key, kv.Value)
	}
	cmd.Println()

	cmd.Println("Properties:")
	if resource.Properties != nil && len(*resource.Properties) > 0 {
		// Extract and sort keys
		keys := make([]string, 0, len(*resource.Properties))
		for key := range *resource.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		// Calculate the maximum key length for properties
		maxPropertyKeyLength := 0
		for _, key := range keys {
			if len(key) > maxPropertyKeyLength {
				maxPropertyKeyLength = len(key)
			}
		}

		// Print each property with aligned keys
		for _, key := range keys {
			value := (*resource.Properties)[key]
			cmd.Printf("  %-*s : %-30v\n", maxPropertyKeyLength, key, value)
		}
	} else {
		cmd.Printf("  -\n")
	}
	cmd.Println()

	status := "-"
	if resource.Status != nil {
		status = *resource.Status
	}
	cmd.Printf("%-*s: %-30s\n", len("Status"), "Status", status)

	return nil
}
