package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/config"
	"github.com/tempestdx/cli/internal/runner"
	appv1 "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1"
	"github.com/tidwall/pretty"
	"google.golang.org/protobuf/types/known/structpb"
)

var (
	testOperation            string
	testInput                string
	testType                 string
	testParentExternalId     string
	testExternalID           string
	testDatasourceInput      string
	testProjectID            string
	testEnvironmentVariables []string

	testCmd = &cobra.Command{
		Use:           "test <app-id>:<app-version>",
		Short:         "Test an app locally.",
		Long:          `The test command is used to test the functionality of a Tempest App.`,
		Args:          cobra.ExactArgs(1),
		RunE:          testRunE,
		SilenceErrors: true,
	}
)

func init() {
	appCmd.AddCommand(testCmd)

	testCmd.Flags().StringVarP(&testOperation, "operation", "o", "", "(REQUIRED) The operation to test. Accepted values: 'create', 'update', 'delete', 'list', 'read'.")
	testCmd.Flags().StringVarP(&testType, "type", "t", "", "(REQUIRED) The type of the resource to test.")

	testCmd.Flags().StringVarP(&testInput, "input", "i", "", "The input to the operation. JSON formatted input options to the operation.")
	testCmd.Flags().StringArrayVar(&testEnvironmentVariables, "env", nil, "Environment variables to set for the operation. Format: KEY=VALUE.")
	testCmd.Flags().StringVarP(&testParentExternalId, "parent-external-id", "p", "", "The external ID of the parent resource. Only required when testing sub-resources.")
	testCmd.Flags().StringVarP(&testExternalID, "external-id", "e", "", "The external ID of the resource to test. Only required when testing 'update', 'delete', or 'read' operations.")

	testCmd.Flags().StringVar(&testProjectID, "project-id", "", "The project ID to use for the operation. If not specified, a random one will be generated.")
	testCmd.Flags().StringVar(&testDatasourceInput, "datasource-input", "", "The datasource input for the 'list' operation.")
}

func testRunE(cmd *cobra.Command, args []string) error {
	id, version, err := splitAppVersion(args[0])
	if err != nil {
		return err
	}

	cfg, cfgDir, err := config.ReadConfig()
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	appVersion := cfg.LookupAppByVersion(id, version)
	if appVersion == nil {
		return fmt.Errorf("app version %s:%s not found in config", id, version)
	}

	if !appPreserveBuildDir {
		err := generateBuildDir(cfg, cfgDir, id, version)
		if err != nil {
			return fmt.Errorf("generate build dir: %w", err)
		}
	}

	runner, cancel, err := runner.StartApp(context.Background(), cfg, cfgDir, id, appVersion)
	if err != nil {
		return fmt.Errorf("start app: %w", err)
	}
	defer cancel()

	des, err := runner.Client.Describe(context.TODO(), connect.NewRequest(&appv1.DescribeRequest{}))
	if err != nil {
		return fmt.Errorf("reach private app: %w", err)
	}

	typesToOperations := make(map[string][]string)
	for _, r := range des.Msg.ResourceDefinitions {
		typesToOperations[r.Type] = []string{}

		if r.ListSupported {
			typesToOperations[r.Type] = append(typesToOperations[r.Type], "list")
		}
		if r.ReadSupported {
			typesToOperations[r.Type] = append(typesToOperations[r.Type], "read")
		}
		if r.CreateSupported {
			typesToOperations[r.Type] = append(typesToOperations[r.Type], "create")
		}
		if r.UpdateSupported {
			typesToOperations[r.Type] = append(typesToOperations[r.Type], "update")
		}
		if r.DeleteSupported {
			typesToOperations[r.Type] = append(typesToOperations[r.Type], "delete")
		}
	}

	if testType == "" {
		return fmt.Errorf("type is required. Available types: %s", strings.Join(slices.Sorted(maps.Keys(typesToOperations)), ", "))
	} else {
		if _, ok := typesToOperations[testType]; !ok {
			return fmt.Errorf("type %s not found in app. Available types: %s", testType, strings.Join(slices.Sorted(maps.Keys(typesToOperations)), ", "))
		}
	}

	if testOperation == "" {
		availableOperations := typesToOperations[testType]
		return fmt.Errorf("operation is required. Supported operations for %s: %s", testType, strings.Join(availableOperations, ", "))
	} else if !slices.Contains(typesToOperations[testType], testOperation) {
		return fmt.Errorf("operation %s not found for type %s. Supported operations: %s", testOperation, testType, strings.Join(typesToOperations[testType], ", "))
	}

	ev := make([]*appv1.EnvironmentVariable, 0, len(testEnvironmentVariables))
	for _, e := range testEnvironmentVariables {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			return fmt.Errorf("invalid environment variable: %s", e)
		}

		ev = append(ev, &appv1.EnvironmentVariable{
			Key:   k,
			Value: v,
			Type:  appv1.EnvironmentVariableType_ENVIRONMENT_VARIABLE_TYPE_VAR,
		})
	}

	switch testOperation {
	case "create":
		req := &appv1.ExecuteResourceOperationRequest{
			Operation: appv1.ResourceOperation_RESOURCE_OPERATION_CREATE,
			Resource: &appv1.Resource{
				Type: testType,
			},
			Metadata: &appv1.Metadata{
				ProjectId: projectID(testProjectID),
			},
			EnvironmentVariables: ev,
		}

		if testInput != "" {
			var input map[string]any
			err := json.Unmarshal([]byte(testInput), &input)
			if err != nil {
				// TODO: provide a better error message, including the required fields from the schema
				return fmt.Errorf("unmarshal input: %w", err)
			}

			s, err := structpb.NewStruct(input)
			if err != nil {
				return fmt.Errorf("new struct: %w", err)
			}

			req.Input = s
		}

		res, err := runner.Client.ExecuteResourceOperation(context.TODO(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("execute resource operation: %w", err)
		}

		cmd.Println("\nResource created with ID:", res.Msg.Resource.GetExternalId())

		j, err := json.MarshalIndent(res.Msg.Resource.Properties, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		cmd.Printf("Properties:\n%s\n", pretty.Color(j, nil))

	case "update":
		if testExternalID == "" {
			return fmt.Errorf("external ID (--external-id) is required for update operation")
		}

		req := &appv1.ExecuteResourceOperationRequest{
			Operation: appv1.ResourceOperation_RESOURCE_OPERATION_UPDATE,
			Resource: &appv1.Resource{
				Type:       testType,
				ExternalId: testExternalID,
			},
			Metadata: &appv1.Metadata{
				ProjectId: projectID(testProjectID),
			},
			EnvironmentVariables: ev,
		}

		if testInput != "" {
			var input map[string]any
			err := json.Unmarshal([]byte(testInput), &input)
			if err != nil {
				// TODO: provide a better error message, including the required fields from the schema
				return fmt.Errorf("unmarshal input: %w", err)
			}

			s, err := structpb.NewStruct(input)
			if err != nil {
				return fmt.Errorf("new struct: %w", err)
			}

			req.Input = s
		}

		res, err := runner.Client.ExecuteResourceOperation(context.TODO(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("execute resource operation: %w", err)
		}

		cmd.Println("\nResource updated with ID:", res.Msg.Resource.GetExternalId())

		j, err := json.MarshalIndent(res.Msg.Resource.Properties, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal output: %w", err)
		}
		cmd.Printf("Properties:\n%s\n", pretty.Color(j, nil))

	case "delete":
		if testExternalID == "" {
			return fmt.Errorf("external ID (--external-id) is required for destroy operation")
		}

		res, err := runner.Client.ExecuteResourceOperation(context.TODO(), connect.NewRequest(&appv1.ExecuteResourceOperationRequest{
			Operation: appv1.ResourceOperation_RESOURCE_OPERATION_DELETE,
			Resource: &appv1.Resource{
				Type:       testType,
				ExternalId: testExternalID,
			},
			Metadata: &appv1.Metadata{
				ProjectId: projectID(testProjectID),
			},
			EnvironmentVariables: ev,
		}))
		if err != nil {
			return fmt.Errorf("execute resource operation: %w", err)
		}

		cmd.Println("Resource deleted with ID:", res.Msg.Resource.GetExternalId())

	case "list":
		var next string
		var resources []*appv1.Resource
		for {
			req := &appv1.ListResourcesRequest{
				Resource: &appv1.Resource{
					Type: testType,
				},
				Metadata: &appv1.Metadata{
					ProjectId: projectID(testProjectID),
				},
				Next: next,
			}

			res, err := runner.Client.ListResources(context.TODO(), connect.NewRequest(req))
			if err != nil {
				return fmt.Errorf("list resources: %w", err)
			}

			resources = append(resources, res.Msg.GetResources()...)

			if res.Msg.Next == "" {
				break
			}

			next = res.Msg.Next
		}

		cmd.Println("Resources:")
		for _, r := range resources {
			j, err := json.MarshalIndent(r.Properties, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal output: %w", err)
			}

			cmd.Println("\nExternal ID:", r.GetExternalId())
			cmd.Printf("Properties:\n%s\n", pretty.Color(j, nil))
		}
	case "read":
		if testExternalID == "" {
			return fmt.Errorf("external ID (--external-id) is required for get operation")
		}

		req := &appv1.ExecuteResourceOperationRequest{
			Operation: appv1.ResourceOperation_RESOURCE_OPERATION_READ,
			Resource: &appv1.Resource{
				Type:       testType,
				ExternalId: testExternalID,
			},
			Metadata: &appv1.Metadata{
				ProjectId: projectID(testProjectID),
			},
			EnvironmentVariables: ev,
		}

		res, err := runner.Client.ExecuteResourceOperation(context.TODO(), connect.NewRequest(req))
		if err != nil {
			return fmt.Errorf("get resource: %w", err)
		}

		j, err := json.MarshalIndent(res.Msg.Resource.Properties, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal resource properties: %w", err)
		}

		cmd.Println("\nResource:", res.Msg.Resource.GetExternalId())
		cmd.Printf("Properties:\n%s\n", pretty.Color(j, nil))
	}

	return nil
}

// projectid is a helper function that will generate a random project ID if one is not provided.
func projectID(id string) string {
	if id != "" {
		return id
	}

	const seed = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
	b := make([]byte, 8)
	for i := range b {
		b[i] = seed[rand.Int()%len(seed)]
	}
	return "TEMPESTCLI" + string(b)
}
