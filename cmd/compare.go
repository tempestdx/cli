package cmd

import (
	"context"
	"fmt"
	"slices"

	"connectrpc.com/connect"
	"github.com/charmbracelet/glamour"
	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/config"
	"github.com/tempestdx/cli/internal/runner"
	appv1 "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1"
	"google.golang.org/protobuf/types/known/structpb"
)

var compareCmd = &cobra.Command{
	Use:   "compare <app_id:app_version_1> <app_id:app_version_2>",
	Short: "Generates a diff of capabilities and operation schemas",
	Args:  cobra.ExactArgs(2),
	RunE:  compareRunE,
}

func init() {
	appCmd.AddCommand(compareCmd)
}

type tableRecord struct {
	resource  string
	operation string
	colA      string
	colB      string
}

func compareRunE(cmd *cobra.Command, args []string) error {
	app1Description, err := getAppVersionDescriptor(args[0])
	if err != nil {
		return fmt.Errorf("get app version descriptor: %w", err)
	}
	app2Description, err := getAppVersionDescriptor(args[1])
	if err != nil {
		return fmt.Errorf("get app version descriptor: %w", err)
	}

	var table string
	table = "Resource | Operation | " + args[0] + " | " + args[1] + "\n"
	table += "-------- | --------- | ------ | ------\n"

	tableRecords := make([]tableRecord, 0)
	// processed app1 resources
	var processedApp1Resources []string
	for _, app1resource := range app1Description.ResourceDefinitions {
		processedApp1Resources = append(processedApp1Resources, app1resource.Type)
		app2resource := lookupResourceByType(app2Description.ResourceDefinitions, app1resource.Type)
		if app2resource == nil {
			// if resource is not present in app2, then it is a removed resource
			tableRecords = append(tableRecords, tableRecord{
				resource: app1resource.Type,
				colA:     boolToCheckmark(true) + " Resource supported",
				colB:     boolToCheckmark(false) + " Resource support removed",
			})
			continue
		}
		tableRecords = append(tableRecords, tableRecord{
			resource: app1resource.Type,
		})

		// resource is present in both app1 and app2, so compare the operations
		// create
		tableRecords = append(tableRecords, compareResourceOperations(
			"create",
			app1resource.CreateSupported,
			app2resource.CreateSupported,
			app1resource.CreateInputSchema,
			app2resource.CreateInputSchema)...)
		// read
		tableRecords = append(tableRecords, compareResourceOperations(
			"read",
			app1resource.ReadSupported,
			app2resource.ReadSupported,
			nil,
			nil)...)
		// update
		tableRecords = append(tableRecords, compareResourceOperations(
			"update",
			app1resource.UpdateSupported,
			app2resource.UpdateSupported,
			app1resource.UpdateInputSchema,
			app2resource.UpdateInputSchema)...)
		// delete
		tableRecords = append(tableRecords, compareResourceOperations(
			"delete",
			app1resource.DeleteSupported,
			app2resource.DeleteSupported,
			nil,
			nil)...)
		// list
		tableRecords = append(tableRecords, compareResourceOperations(
			"list",
			app1resource.ListSupported,
			app2resource.ListSupported,
			nil,
			nil)...)
		// healthcheck
		tableRecords = append(tableRecords, compareResourceOperations(
			"healthcheck",
			app1resource.HealthcheckSupported,
			app2resource.HealthcheckSupported,
			nil,
			nil)...)
	}
	for _, app2resource := range app2Description.ResourceDefinitions {
		if !slices.Contains(processedApp1Resources, app2resource.Type) {
			// if resource is not present in app1, then it is an added resource
			tableRecords = append(tableRecords, tableRecord{
				resource: app2resource.Type,
				colA:     boolToCheckmark(false) + " Resource not supported",
				colB:     boolToCheckmark(true) + " Resource support added",
			})
		}
	}

	for i, v := range tableRecords {
		if v.resource != "" || v.operation != "" {
			if i > 0 && tableRecords[i-1].operation == v.operation {
				table += "|" + v.resource + " || " + v.colA + " | " + v.colB + "|\n"
			} else {
				table += "|" + v.resource + " | " + v.operation + " | " + v.colA + " | " + v.colB + "|\n"
			}
		}
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
	cmd.Println(out)
	return nil
}

func getAppVersionDescriptor(appNameVersion string) (*appv1.DescribeResponse, error) {
	id, version, err := splitAppVersion(appNameVersion)
	if err != nil {
		return nil, err
	}

	cfg, cfgDir, err := config.ReadConfig()
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	appVersion := cfg.LookupAppByVersion(id, version)
	if appVersion == nil {
		return nil, fmt.Errorf("app %s:%s not found", id, version)
	}

	err = generateBuildDir(cfg, cfgDir)
	if err != nil {
		return nil, fmt.Errorf("generate build dir: %w", err)
	}

	runners, cancel, err := runner.StartApps(context.TODO(), cfg, cfgDir)
	if err != nil {
		return nil, fmt.Errorf("start local app: %w", err)
	}
	defer cancel()

	var runner runner.Runner
	for _, r := range runners {
		if r.AppID == id && r.Version == version {
			runner = r
			break
		}
	}

	res, err := runner.Client.Describe(context.TODO(), connect.NewRequest(&appv1.DescribeRequest{}))
	if err != nil {
		return nil, fmt.Errorf("reach private app: %w", err)
	}

	return res.Msg, nil
}

func lookupResourceByType(resources []*appv1.ResourceDefinition, resourceType string) *appv1.ResourceDefinition {
	for _, resource := range resources {
		if resource.Type == resourceType {
			return resource
		}
	}
	return nil
}

func compareResourceOperations(operation string, app1ResSupport, app2ResSupport bool, app1ResSchema, app2ResSchema *structpb.Struct) []tableRecord {
	switch {
	case app1ResSupport && app2ResSupport:
		// both app1 and app2 support operation
		// compare the schemas
		schemaDiffRecords := compareResourceSchemas(operation, app1ResSchema, app2ResSchema)
		if len(schemaDiffRecords) > 0 {
			// if there are schema differences, add the operation record
			return append([]tableRecord{
				{
					operation: operation,
					colB:      "Properties differ",
				},
			}, schemaDiffRecords...)
		}
	case app1ResSupport && !app2ResSupport:
		// app1 supports operation, but app2 does not
		// print the removed operation
		return []tableRecord{
			{
				operation: operation,
				colA:      boolToCheckmark(true) + " Operation supported",
				colB:      boolToCheckmark(false) + " Operation support removed",
			},
		}
	case !app1ResSupport && app2ResSupport:
		// app2 supports operation, but app1 does not
		// print the added operation
		return []tableRecord{
			{
				operation: operation,
				colA:      boolToCheckmark(false) + " Operation not supported",
				colB:      boolToCheckmark(true) + " Operation support added",
			},
		}
	}

	return []tableRecord{}
}

func compareResourceSchemas(operation string, app1ResSchema, app2ResSchema *structpb.Struct) []tableRecord {
	records := make([]tableRecord, 0)
	// compare the schemas
	if app1ResSchema == nil && app2ResSchema == nil {
		// both schemas are nil so no comparison needed
		return records
	}
	var app1SchemaSeenFields []string

	app1Properties := app1ResSchema.Fields["properties"].GetStructValue()
	app2Properties := app2ResSchema.Fields["properties"].GetStructValue()

	var (
		colA string
		colB string
	)
	if app1Properties == nil {
		colA = "No properties"
	}
	if app2Properties == nil {
		colB = "No properties"
	}
	if colA != "" || colB != "" {
		// if one of the schemas is nil, then everything is different
		records = append(records, tableRecord{
			operation: operation,
			colA:      colA,
			colB:      colB,
		})
		return records
	}

	for k := range app1Properties.Fields {
		app1SchemaSeenFields = append(app1SchemaSeenFields, k)

		if _, ok := app2Properties.Fields[k]; !ok {
			// field is present in app1 schema but not in app2 schema
			records = append(records, tableRecord{
				operation: operation,
				colA:      fmt.Sprintf("%s %s property", boolToCheckmark(true), k),
				colB:      fmt.Sprintf("%s %s property", boolToCheckmark(false), k),
			})
		}
	}
	for k := range app2Properties.Fields {
		if !slices.Contains(app1SchemaSeenFields, k) {
			// field is present in app2 schema but not in app1 schema
			records = append(records, tableRecord{
				operation: operation,
				colA:      fmt.Sprintf("%s %s property", boolToCheckmark(false), k),
				colB:      fmt.Sprintf("%s %s property", boolToCheckmark(true), k),
			})
		}
	}

	return records
}
