package cmd

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/config"
	"github.com/tempestdx/cli/internal/runner"
	"github.com/tempestdx/cli/internal/secret"
	appapi "github.com/tempestdx/openapi/app"
	appv1 "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1"
	appv1connect "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1/appv1connect"
	"github.com/zalando/go-keyring"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	TempestProdAPI  = "https://developer.tempestdx.com/api/v1"
	pollingInterval = 5 * time.Second
)

var (
	appServeHealthcheckInterval time.Duration
	token                       string

	serveCmd = &cobra.Command{
		Use:   "serve [<app-id>:<app-version>]",
		Short: "Facilitates the serving of Tempest apps.",
		Long: `The serve command is used to start your Tempest apps and orchestrate commands from the Tempest API.

If no app ID and version is provided, it will serve all apps from the tempest.yaml configuration file.`,
		Args: cobra.RangeArgs(0, 1),
		RunE: serveRunE,
	}
)

func init() {
	appCmd.AddCommand(serveCmd)

	serveCmd.Flags().DurationVarP(&appServeHealthcheckInterval, "healthcheck-interval", "i", 5*time.Minute, "The interval at which to perform healthchecks.")
}

func serveRunE(cmd *cobra.Command, args []string) error {
	var id, version string
	if len(args) > 0 {
		var err error
		id, version, err = splitAppVersion(args[0])
		if err != nil {
			return err
		}
	}

	token = os.Getenv("TEMPEST_TOKEN")
	if token == "" {
		var err error
		token, err = tokenStore.Get()
		if err != nil {
			if errors.Is(err, keyring.ErrNotFound) {
				return fmt.Errorf("token not found. Please login with 'tempest auth login' or set the TEMPEST_TOKEN environment variable")
			}
			return fmt.Errorf("get token: %w", err)
		}
	}

	cfg, cfgDir, err := config.ReadConfig()
	if err != nil {
		return err
	}

	if !appPreserveBuildDir {
		err := generateBuildDir(cfg, cfgDir)
		if err != nil {
			return fmt.Errorf("generate build dir: %w", err)
		}
	}

	runners, cancel, err := runner.StartApps(context.TODO(), cfg, cfgDir)
	if err != nil {
		return fmt.Errorf("start local app: %w", err)
	}
	defer cancel()

	waveClient, err := appapi.NewClientWithResponses(
		apiEndpoint,
		appapi.WithHTTPClient(&http.Client{
			Timeout:   10 * time.Second,
			Transport: secret.NewTransportWithToken(token),
		}),
	)
	if err != nil {
		return err
	}

	if id != "" && version != "" {
		var runner runner.Runner
		for _, r := range runners {
			if r.AppID == id && r.Version == version {
				runner = r
				break
			}
		}

		go startHealthCheck(cmd, runner, waveClient, appServeHealthcheckInterval)
		go startPolling(cmd, runner, waveClient)
	} else {
		for _, runner := range runners {
			go startHealthCheck(cmd, runner, waveClient, appServeHealthcheckInterval)
			go startPolling(cmd, runner, waveClient)
		}
	}

	// wait for ctrl+c
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP)

	<-signalChan

	return nil
}

func startPolling(cmd *cobra.Command, runner runner.Runner, waveClient *appapi.ClientWithResponses) {
	cmd.Println("Starting polling for app: ", runner.AppID+":"+runner.Version)

	for {
		cmd.Printf("app %s: polling\n", runner.AppID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		nextTask, err := waveClient.PostAppsOperationsNextWithResponse(ctx, appapi.PostAppsOperationsNextJSONRequestBody{
			AppId:   runner.AppID,
			Version: runner.Version,
		})
		cancel()
		if err != nil {
			cmd.PrintErrf("app %s: failed to get next task: %s. Will retry\n", runner.AppID, err)
			time.Sleep(pollingInterval)
			continue
		}

		cmd.Printf("app %s: got response\n", runner.AppID)

		cmd.Printf("app %s: %s\n", runner.AppID, nextTask.Status())
		switch nextTask.StatusCode() {
		case http.StatusOK:
			val, err := nextTask.JSON200.Task.ValueByDiscriminator()
			if err != nil {
				cmd.PrintErrf("app %s: fail to unpack next task: %s\n", runner.AppID, err)
				time.Sleep(pollingInterval)
				continue
			}

			switch v := val.(type) {
			case appapi.ExecuteResourceOperationRequest:
				cmd.Printf("app %s: executing resource operation %s\n", runner.AppID, v.Operation)

				input, err := structpb.NewStruct(*v.Input)
				if err != nil {
					cmd.PrintErrf("app %s: %s\n", runner.AppID, err)
					time.Sleep(pollingInterval)
					continue
				}

				var op appv1.ResourceOperation
				switch v.Operation {
				case appapi.Create:
					op = appv1.ResourceOperation_RESOURCE_OPERATION_CREATE
				case appapi.Update:
					op = appv1.ResourceOperation_RESOURCE_OPERATION_UPDATE
				case appapi.Delete:
					op = appv1.ResourceOperation_RESOURCE_OPERATION_DELETE
				case appapi.Read:
					op = appv1.ResourceOperation_RESOURCE_OPERATION_READ
				default:
					cmd.PrintErrf("app %s: unsupported operation: %s\n", runner.AppID, v.Operation)
					time.Sleep(pollingInterval)
					continue
				}

				metadata := &appv1.Metadata{
					ProjectId:   nextTask.JSON200.Metadata.ProjectId,
					ProjectName: nextTask.JSON200.Metadata.ProjectName,
					Author:      waveOwnerToAppOwner(nextTask.JSON200.Metadata.Author),
					Owners:      make([]*appv1.Owner, 0, len(nextTask.JSON200.Metadata.Owners)),
				}
				for _, owner := range nextTask.JSON200.Metadata.Owners {
					metadata.Owners = append(metadata.Owners, waveOwnerToAppOwner(owner))
				}

				environment := []*appv1.EnvironmentVariable{}
				if v.EnvironmentVariables != nil {
					for _, env := range *v.EnvironmentVariables {
						var envType appv1.EnvironmentVariableType
						switch env.Type {
						case "variable":
							envType = appv1.EnvironmentVariableType_ENVIRONMENT_VARIABLE_TYPE_VAR
						case "secret":
							envType = appv1.EnvironmentVariableType_ENVIRONMENT_VARIABLE_TYPE_SECRET
						case "certificate":
							envType = appv1.EnvironmentVariableType_ENVIRONMENT_VARIABLE_TYPE_CERTIFICATE
						case "private_key":
							envType = appv1.EnvironmentVariableType_ENVIRONMENT_VARIABLE_TYPE_PRIVATE_KEY
						case "public_key":
							envType = appv1.EnvironmentVariableType_ENVIRONMENT_VARIABLE_TYPE_PUBLIC_KEY
						default:
							envType = appv1.EnvironmentVariableType_ENVIRONMENT_VARIABLE_TYPE_UNSPECIFIED
						}

						environment = append(environment, &appv1.EnvironmentVariable{
							Key:   env.Name,
							Value: env.Value,
							Type:  envType,
						})
					}
				}

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				res, err := runner.Client.ExecuteResourceOperation(ctx, connect.NewRequest(&appv1.ExecuteResourceOperationRequest{
					Resource: &appv1.Resource{
						Type:       v.Resource.Type,
						ExternalId: v.Resource.ExternalId,
					},
					Operation:            op,
					Input:                input,
					Metadata:             metadata,
					EnvironmentVariables: environment,
				}))
				cancel()
				if err != nil {
					if waveErr := postWaveError(waveClient, nextTask.JSON200.TaskId, err); waveErr != nil {
						cmd.PrintErrf("app %s: report task %s: %s\n", runner.AppID, nextTask.JSON200.TaskId, waveErr)
					}
					cmd.PrintErrf("app %s: execute operation: %s\n", runner.AppID, err)
					time.Sleep(pollingInterval)
					continue
				}

				cmd.Printf("app %s: executed. Output: %v\n", runner.AppID, res)

				// prepare the response depending on the operation
				var response appapi.ReportResponse_Response

				resource := appapi.Resource{
					Type:        res.Msg.Resource.Type,
					ExternalId:  res.Msg.Resource.ExternalId,
					DisplayName: res.Msg.Resource.DisplayName,
				}

				properties := res.Msg.Resource.Properties.AsMap()
				resource.Properties = &properties

				items := make([]appapi.LinksItem, 0, len(res.Msg.Resource.Links))
				for _, link := range res.Msg.Resource.Links {
					items = append(items, appapi.LinksItem{
						Title: link.Title,
						Url:   link.Url,
						Type:  appapi.LinksItemType(link.Type.String()),
					})
				}

				resource.Links = &appapi.Links{
					Links: &items,
				}

				err = response.MergeExecuteResourceOperationResponse(appapi.ExecuteResourceOperationResponse{
					Resource:     &resource,
					ResponseType: "execute_resource_operation",
				})
				if err != nil {
					cmd.PrintErrf("app %s: prepare app response: %s\n", runner.AppID, err)
					time.Sleep(pollingInterval)
					continue
				}

				// post the response to the wave api
				cmd.Printf("app %s: posting response to wave api\n", runner.AppID)
				_, err = waveClient.PostAppsOperationsReport(context.TODO(), appapi.PostAppsOperationsReportJSONRequestBody{
					TaskId:   nextTask.JSON200.TaskId,
					Response: response,
					Status:   appapi.ReportResponseStatusOk,
				})
				if err != nil {
					cmd.PrintErrf("app %s: post app response: %s\n", runner.AppID, err)
					time.Sleep(pollingInterval)
					continue
				}
				fmt.Printf("app %s: success!\n", runner.AppID)
			case appapi.ExecuteResourceActionRequest:
				// TODO
			case appapi.ListResourcesRequest:
				cmd.Printf("app %s: listing resources\n", runner.AppID)

				metadata := &appv1.Metadata{
					ProjectId:   nextTask.JSON200.Metadata.ProjectId,
					ProjectName: nextTask.JSON200.Metadata.ProjectName,
					Author:      waveOwnerToAppOwner(nextTask.JSON200.Metadata.Author),
					Owners:      make([]*appv1.Owner, 0, len(nextTask.JSON200.Metadata.Owners)),
				}
				for _, owner := range nextTask.JSON200.Metadata.Owners {
					metadata.Owners = append(metadata.Owners, waveOwnerToAppOwner(owner))
				}

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				res, err := runner.Client.ListResources(ctx, connect.NewRequest(&appv1.ListResourcesRequest{
					Resource: &appv1.Resource{
						Type: v.Resource.Type,
					},
					Next:     v.Next,
					Metadata: metadata,
				}))
				cancel()
				if err != nil {
					if waveErr := postWaveError(waveClient, nextTask.JSON200.TaskId, err); waveErr != nil {
						cmd.PrintErrf("app %s: report task %s: %s\n", runner.AppID, nextTask.JSON200.TaskId, waveErr)
					}
					cmd.PrintErrf("app %s: execute list resources: %s\n", runner.AppID, err)
					time.Sleep(pollingInterval)
					continue
				}

				resources := make([]appapi.Resource, len(res.Msg.Resources))
				for i, r := range res.Msg.Resources {
					properties := r.Properties.AsMap()
					items := make([]appapi.LinksItem, 0, len(r.Links))
					for _, link := range r.Links {
						items = append(items, appapi.LinksItem{
							Title: link.Title,
							Url:   link.Url,
							Type:  appapi.LinksItemType(link.Type.String()),
						})
					}

					resources[i] = appapi.Resource{
						ExternalId:  r.ExternalId,
						DisplayName: r.DisplayName,
						Properties:  &properties,
						Type:        r.Type,
						Links: &appapi.Links{
							Links: &items,
						},
					}
				}

				var response appapi.ReportResponse_Response
				err = response.MergeListResourcesResponse(appapi.ListResourcesResponse{
					Next:         res.Msg.Next,
					Resources:    resources,
					ResponseType: "list_resources",
				})
				if err != nil {
					cmd.PrintErrf("app %s: prepare app response: %s\n", runner.AppID, err)
					time.Sleep(pollingInterval)
					continue
				}

				// post the response to the wave api
				cmd.Printf("app %s: posting response to wave api\n", runner.AppID)
				_, err = waveClient.PostAppsOperationsReport(context.TODO(), appapi.PostAppsOperationsReportJSONRequestBody{
					TaskId:   nextTask.JSON200.TaskId,
					Response: response,
					Status:   appapi.ReportResponseStatusOk,
				})
				if err != nil {
					cmd.PrintErrf("app %s: post app response: %s\n", runner.AppID, err)
					time.Sleep(pollingInterval)
					continue
				}
				fmt.Printf("app %s: success!\n", runner.AppID)
			}

		case http.StatusNoContent:
			cmd.Printf("app %s: no tasks available, sleeping\n", runner.AppID)
			time.Sleep(pollingInterval)
		case http.StatusInternalServerError:
			cmd.Printf("app %s: internal server error, sleeping\n", runner.AppID)
			time.Sleep(pollingInterval)
		default:
			cmd.Printf("app %s: unexpected status: %s (%d)\n", runner.AppID, nextTask.Status(), nextTask.StatusCode())
		}
	}
}

func startHealthCheck(
	cmd *cobra.Command,
	runner runner.Runner,
	waveClient *appapi.ClientWithResponses,
	interval time.Duration,
) {
	cmd.Println("Starting health check for app: ", runner.AppID+":"+runner.Version)

	des, err := runner.Client.Describe(context.TODO(), connect.NewRequest(&appv1.DescribeRequest{}))
	if err != nil {
		fmt.Println("describe error", err)
	}

	// Send one health check immediately
	err = performHealthCheck(runner.Client, waveClient, des.Msg.ResourceDefinitions, runner.AppID, runner.Version)
	if err != nil {
		cmd.PrintErrf("health check error: %v\n", err)
	}

	// Start the ticker, which will perform health checks at the specified interval
	ticker := time.NewTicker(interval)
	for range ticker.C {
		<-ticker.C
		err := performHealthCheck(runner.Client, waveClient, des.Msg.ResourceDefinitions, runner.AppID, runner.Version)
		if err != nil {
			cmd.PrintErrf("health check error: %v\n", err)
		}
	}
}

func performHealthCheck(
	client appv1connect.AppServiceClient,
	waveClient *appapi.ClientWithResponses,
	types []*appv1.ResourceDefinition,
	appID string,
	appVersion string,
) error {
	var reports []appapi.AppHealthReportItem
	for _, t := range types {
		if !t.HealthcheckSupported {
			continue
		}

		res, err := client.HealthCheck(context.TODO(), connect.NewRequest(&appv1.HealthCheckRequest{
			Type: t.Type,
		}))
		if err != nil {
			return fmt.Errorf("health check error: %w", err)
		}

		if res.Msg.Status != appv1.HealthCheckStatus_HEALTH_CHECK_STATUS_UNSPECIFIED {
			reports = append(reports, appapi.AppHealthReportItem{
				Type:    t.Type,
				Status:  appStatusToWaveStatus(res.Msg.Status),
				Message: &res.Msg.Message,
			})
		}

		_, err = waveClient.PostAppsVersionsHealth(context.TODO(), appapi.PostAppsVersionsHealthJSONRequestBody{
			AppId:         appID,
			Version:       appVersion,
			HealthReports: reports,
		})
		if err != nil {
			return fmt.Errorf("post health reports error: %w", err)
		}
	}

	return nil
}

func appStatusToWaveStatus(status appv1.HealthCheckStatus) appapi.AppHealthReportItemStatus {
	switch status {
	case appv1.HealthCheckStatus_HEALTH_CHECK_STATUS_HEALTHY:
		return appapi.Healthy
	case appv1.HealthCheckStatus_HEALTH_CHECK_STATUS_DEGRADED:
		return appapi.Degraded
	case appv1.HealthCheckStatus_HEALTH_CHECK_STATUS_DISRUPTED:
		return appapi.Disrupted
	default:
		return "unknown"
	}
}

func waveOwnerToAppOwner(owner appapi.Owner) *appv1.Owner {
	var t appv1.OwnerType
	switch owner.Type {
	case appapi.User:
		t = appv1.OwnerType_OWNER_TYPE_USER
	case appapi.Team:
		t = appv1.OwnerType_OWNER_TYPE_TEAM
	}

	return &appv1.Owner{
		Email: owner.Email,
		Name:  owner.Name,
		Type:  t,
	}
}

func postWaveError(waveClient *appapi.ClientWithResponses, taskID string, appErr error) error {
	errStr := appErr.Error()
	_, err := waveClient.PostAppsOperationsReport(context.TODO(), appapi.PostAppsOperationsReportJSONRequestBody{
		TaskId:  taskID,
		Status:  appapi.ReportResponseStatusError,
		Message: &errStr,
	})
	return err
}
