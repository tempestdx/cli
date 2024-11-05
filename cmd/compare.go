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

var emptyRecord = tableRecord{
	resource:  " ",
	operation: " ",
	colA:      " ",
	colB:      " ",
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
	table = "| Resource | Operation | " + args[0] + " | " + args[1] + "\n"
	table += "| -------- | -------- | -------- | -------- |\n"

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
				colA:     fmt.Sprintf("++ resource: %s", app1resource.Type),
				colB:     fmt.Sprintf("-- resource: %s", app1resource.Type),
			}, emptyRecord)
			continue
		}
		tableRecords = append(tableRecords, tableRecord{
			resource: app1resource.Type,
		})

		// resource is present in both app1 and app2, so compare the operations
		// create
		tableRecords = append(tableRecords, compareOperations(
			"create",
			app1resource.CreateSupported,
			app2resource.CreateSupported,
			app1resource.CreateInputSchema,
			app2resource.CreateInputSchema)...)
		// read
		tableRecords = append(tableRecords, compareOperations(
			"read",
			app1resource.ReadSupported,
			app2resource.ReadSupported,
			nil,
			nil)...)
		// update
		tableRecords = append(tableRecords, compareOperations(
			"update",
			app1resource.UpdateSupported,
			app2resource.UpdateSupported,
			app1resource.UpdateInputSchema,
			app2resource.UpdateInputSchema)...)
		// delete
		tableRecords = append(tableRecords, compareOperations(
			"delete",
			app1resource.DeleteSupported,
			app2resource.DeleteSupported,
			nil,
			nil)...)
		// list
		tableRecords = append(tableRecords, compareOperations(
			"list",
			app1resource.ListSupported,
			app2resource.ListSupported,
			nil,
			nil)...)
		// healthcheck
		tableRecords = append(tableRecords, compareOperations(
			"healthcheck",
			app1resource.HealthcheckSupported,
			app2resource.HealthcheckSupported,
			nil,
			nil)...)
		// actions
		var processedApp1Actions []string
		for _, action := range app1resource.Actions {
			processedApp1Actions = append(processedApp1Actions, action.Name)
			for _, app2action := range app2resource.Actions {
				if action.Name == app2action.Name {
					// action is present in both app1 and app2, so compare the operations
					tableRecords = append(tableRecords, compareOperations(
						action.Name,
						true,
						true,
						action.InputSchema,
						app2action.InputSchema)...)
					break
				}
				if len(processedApp1Actions) == len(app1resource.Actions) {
					// action is not present in app2, then it is a removed action
					tableRecords = append(tableRecords, tableRecord{
						operation: action.Name,
						colA:      fmt.Sprintf("++ action: %s", action.Name),
						colB:      fmt.Sprintf("-- action: %s", action.Name),
					}, emptyRecord)
				}
			}
		}
		for _, app2action := range app2resource.Actions {
			if !slices.Contains(processedApp1Actions, app2action.Name) {
				// if action is not present in app1, then it is an added action
				tableRecords = append(tableRecords, tableRecord{
					operation: app2action.Name,
					colA:      fmt.Sprintf("-- action: %s", app2action.Name),
					colB:      fmt.Sprintf("++ action: %s", app2action.Name),
				}, emptyRecord)
			}
		}
	}
	for _, app2resource := range app2Description.ResourceDefinitions {
		if !slices.Contains(processedApp1Resources, app2resource.Type) {
			// if resource is not present in app1, then it is an added resource
			tableRecords = append(tableRecords, tableRecord{
				resource: app2resource.Type,
				colA:     fmt.Sprintf("-- resource: %s", app2resource.Type),
				colB:     fmt.Sprintf("++ resource: %s", app2resource.Type),
			}, emptyRecord)
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

	if !appPreserveBuildDir {
		err := generateBuildDir(cfg, cfgDir, id, version)
		if err != nil {
			return nil, fmt.Errorf("generate build dir: %w", err)
		}
	}

	runner, cancel, err := runner.StartApp(context.TODO(), cfg, cfgDir, id, version)
	if err != nil {
		return nil, fmt.Errorf("start local app: %w", err)
	}
	defer cancel()

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

func compareOperations(operation string, app1Support, app2Support bool, app1Schema, app2Schema *structpb.Struct) []tableRecord {
	switch {
	case app1Support && app2Support:
		// both app1 and app2 support operation
		// compare the schemas
		schemaDiffRecords := compareResourceSchemas(operation, app1Schema, app2Schema)
		if len(schemaDiffRecords) > 0 {
			schemaDiffRecords = append(schemaDiffRecords, emptyRecord)
			// if there are schema differences, add the operation record
			return append([]tableRecord{
				{
					operation: operation,
					colA:      " Schema changed",
				},
			}, schemaDiffRecords...)
		}
	case app1Support && !app2Support:
		// app1 supports operation, but app2 does not
		// print the removed operation
		return []tableRecord{
			{
				operation: operation,
				colA:      fmt.Sprintf("++ operation: %s", operation),
				colB:      fmt.Sprintf("-- operation: %s", operation),
			},
			emptyRecord,
		}
	case !app1Support && app2Support:
		// app2 supports operation, but app1 does not
		// print the added operation
		return []tableRecord{
			{
				operation: operation,
				colA:      fmt.Sprintf("-- operation: %s", operation),
				colB:      fmt.Sprintf("++ operation: %s", operation),
			},
			emptyRecord,
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
				colA:      fmt.Sprintf("++ property: %s", k),
				colB:      fmt.Sprintf("-- property: %s", k),
			})
		}
	}
	for k := range app2Properties.Fields {
		if !slices.Contains(app1SchemaSeenFields, k) {
			// field is present in app2 schema but not in app1 schema
			records = append(records, tableRecord{
				operation: operation,
				colA:      fmt.Sprintf("-- property: %s", k),
				colB:      fmt.Sprintf("++ property: %s", k),
			})
		}
	}

	return records
}
