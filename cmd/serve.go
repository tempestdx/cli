package cmd

import (
	"context"
	"fmt"
	"log/slog"
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
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	TempestProdAPI  = "https://developer.tempestdx.com/api/v1"
	pollingInterval = 5 * time.Second
)

var (
	appServeHealthcheckInterval time.Duration
	appExecutionTimeout         time.Duration
	logger                      *slog.Logger

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
	serveCmd.Flags().DurationVarP(&appExecutionTimeout, "app-execution-timeout", "t", 5*time.Minute, "The timeout for the app execution operation.")
}

func serveRunE(cmd *cobra.Command, args []string) error {
	var logLevel slog.Level = slog.LevelInfo
	if debugMode {
		logLevel = slog.LevelDebug
	}

	logger = slog.New(slog.NewJSONHandler(cmd.OutOrStderr(), &slog.HandlerOptions{
		Level: logLevel,
	}))

	var id, version string
	if len(args) > 0 {
		var err error
		id, version, err = splitAppVersion(args[0])
		if err != nil {
			return err
		}
	}

	token := loadTempestToken(cmd)

	cfg, cfgDir, err := config.ReadConfig()
	if err != nil {
		return err
	}

	tempestClient, err := appapi.NewClientWithResponses(
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

		runner, cancel, err := runner.StartApp(context.TODO(), cfg, cfgDir, id, appVersion)
		if err != nil {
			return fmt.Errorf("start local app: %w", err)
		}
		defer cancel()

		go startHealthCheck(runner, tempestClient, appServeHealthcheckInterval)
		go startPolling(runner, tempestClient)
	} else {
		if !appPreserveBuildDir {
			err := generateBuildDir(cfg, cfgDir, id, version)
			if err != nil {
				return fmt.Errorf("generate build dir: %w", err)
			}
		}

		runners, cancel, err := runner.StartApps(context.TODO(), cfg, cfgDir)
		if err != nil {
			return fmt.Errorf("start local app: %w", err)
		}
		defer cancel()

		for _, runner := range runners {
			go startHealthCheck(runner, tempestClient, appServeHealthcheckInterval)
			go startPolling(runner, tempestClient)
		}
	}

	// wait for ctrl+c
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGHUP)

	<-signalChan

	return nil
}

func startPolling(runner runner.Runner, tempestClient *appapi.ClientWithResponses) {
	logger := logger.With("app_id", runner.AppID, "version", runner.Version)

	logger.Info("start polling")

	for {
		logger.Debug("polling for next task")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		nextTask, err := tempestClient.PostAppsOperationsNextWithResponse(ctx, appapi.PostAppsOperationsNextJSONRequestBody{
			AppId:   runner.AppID,
			Version: runner.Version,
		})
		cancel()
		if err != nil {
			logger.Error("failed to get next task. Will retry", "error", err)
			time.Sleep(pollingInterval)
			continue
		}

		logger.Debug("got response", "status", nextTask.Status(), "code", nextTask.StatusCode())
		switch nextTask.StatusCode() {
		case http.StatusOK:
			val, err := nextTask.JSON200.Task.ValueByDiscriminator()
			if err != nil {
				logger.Error("fail to unpack next task", "error", err)
				time.Sleep(pollingInterval)
				continue
			}

			switch v := val.(type) {
			case appapi.ExecuteResourceOperationRequest:
				logger.Info("executing resource operation", "operation", v.Operation)

				input, err := structpb.NewStruct(*v.Input)
				if err != nil {
					logger.Error("prepare operation request fail", "error", err)
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
					logger.Error("unsupported operation", "operation", v.Operation)
					time.Sleep(pollingInterval)
					continue
				}

				metadata := &appv1.Metadata{
					ProjectId:   nextTask.JSON200.Metadata.ProjectId,
					ProjectName: nextTask.JSON200.Metadata.ProjectName,
					Author:      tempestOwnerToAppOwner(nextTask.JSON200.Metadata.Author),
					Owners:      make([]*appv1.Owner, 0, len(nextTask.JSON200.Metadata.Owners)),
				}
				for _, owner := range nextTask.JSON200.Metadata.Owners {
					metadata.Owners = append(metadata.Owners, tempestOwnerToAppOwner(owner))
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

				ctx, cancel := context.WithTimeout(context.Background(), appExecutionTimeout)
				res, err := runner.Client.ExecuteResourceOperation(ctx, connect.NewRequest(&appv1.ExecuteResourceOperationRequest{
					Resource: &appv1.Resource{
						Type:       *v.Resource.Type,
						ExternalId: v.Resource.ExternalId,
					},
					Operation:            op,
					Input:                input,
					Metadata:             metadata,
					EnvironmentVariables: environment,
				}))
				cancel()
				if err != nil {
					if tempestErr := postTempestError(tempestClient, nextTask.JSON200.TaskId, err); tempestErr != nil {
						logger.Error("report task", "task_id", nextTask.JSON200.TaskId, "error", tempestErr)
					}
					logger.Error("execute operation", "error", err)
					time.Sleep(pollingInterval)
					continue
				}

				logger.Debug("app operation executed", "output", res)

				// prepare the response depending on the operation
				var response appapi.ReportResponse_Response

				resource := appapi.Resource{
					Type:        &res.Msg.Resource.Type,
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
					logger.Error("prepare app response", "error", err)
					time.Sleep(pollingInterval)
					continue
				}

				// post the response to the Tempest API
				logger.Info("posting response to Tempest API")
				_, err = tempestClient.PostAppsOperationsReport(context.TODO(), appapi.PostAppsOperationsReportJSONRequestBody{
					TaskId:   nextTask.JSON200.TaskId,
					Response: response,
					Status:   appapi.ReportResponseStatusOk,
				})
				if err != nil {
					logger.Error("post app response", "error", err)
					time.Sleep(pollingInterval)
					continue
				}
				logger.Info("post app response successful")
			case appapi.ExecuteResourceActionRequest:
				// TODO
			case appapi.ListResourcesRequest:
				logger.Info("listing resources")

				metadata := &appv1.Metadata{
					ProjectId:   nextTask.JSON200.Metadata.ProjectId,
					ProjectName: nextTask.JSON200.Metadata.ProjectName,
					Author:      tempestOwnerToAppOwner(nextTask.JSON200.Metadata.Author),
					Owners:      make([]*appv1.Owner, 0, len(nextTask.JSON200.Metadata.Owners)),
				}
				for _, owner := range nextTask.JSON200.Metadata.Owners {
					metadata.Owners = append(metadata.Owners, tempestOwnerToAppOwner(owner))
				}

				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				res, err := runner.Client.ListResources(ctx, connect.NewRequest(&appv1.ListResourcesRequest{
					Resource: &appv1.Resource{
						Type: *v.Resource.Type,
					},
					Next:     v.Next,
					Metadata: metadata,
				}))
				cancel()
				if err != nil {
					if tempestErr := postTempestError(tempestClient, nextTask.JSON200.TaskId, err); tempestErr != nil {
						logger.Error("report task", "task_id", nextTask.JSON200.TaskId, "error ", tempestErr)
					}
					logger.Error("execute list resources", "error", err)
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
						Type:        &r.Type,
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
					logger.Error("prepare app response", "error", err)
					time.Sleep(pollingInterval)
					continue
				}

				// post the response to the Tempest API
				logger.Info("posting response to Tempest API")
				_, err = tempestClient.PostAppsOperationsReport(context.TODO(), appapi.PostAppsOperationsReportJSONRequestBody{
					TaskId:   nextTask.JSON200.TaskId,
					Response: response,
					Status:   appapi.ReportResponseStatusOk,
				})
				if err != nil {
					logger.Error("post app response", "error", err)
					time.Sleep(pollingInterval)
					continue
				}
				logger.Info("post app response successful")
			}

		case http.StatusNoContent:
			logger.Debug("no tasks available, sleeping")
			time.Sleep(pollingInterval)
		case http.StatusInternalServerError:
			logger.Error("internal server error, sleeping")
			time.Sleep(pollingInterval)
		case http.StatusUnauthorized:
			logger.Error("unauthorized, expired/revoked token")
			time.Sleep(pollingInterval)
		default:
			logger.Error("unexpected status", "status", nextTask.Status(), "status_code", nextTask.StatusCode())
			time.Sleep(pollingInterval)
		}
	}
}

func startHealthCheck(
	runner runner.Runner,
	tempestClient *appapi.ClientWithResponses,
	interval time.Duration,
) {
	logger := logger.With("app_id", runner.AppID, "version", runner.Version)

	logger.Info("starting health check")

	des, err := runner.Client.Describe(context.TODO(), connect.NewRequest(&appv1.DescribeRequest{}))
	if err != nil {
		logger.Error("describe app", "error", err)
	}

	// Send one health check immediately
	err = performHealthCheck(runner.Client, tempestClient, des.Msg.ResourceDefinitions, runner.AppID, runner.Version)
	if err != nil {
		logger.Error("health check", "error", err)
	}

	// Start the ticker, which will perform health checks at the specified interval
	ticker := time.NewTicker(interval)
	for range ticker.C {
		<-ticker.C
		err := performHealthCheck(runner.Client, tempestClient, des.Msg.ResourceDefinitions, runner.AppID, runner.Version)
		if err != nil {
			logger.Error("health check", "error", err)
		}
	}
}

func performHealthCheck(
	client appv1connect.AppServiceClient,
	tempestClient *appapi.ClientWithResponses,
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
				Status:  appStatusToTempestStatus(res.Msg.Status),
				Message: &res.Msg.Message,
			})
		}

		_, err = tempestClient.PostAppsVersionsHealth(context.TODO(), appapi.PostAppsVersionsHealthJSONRequestBody{
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

func appStatusToTempestStatus(status appv1.HealthCheckStatus) appapi.AppHealthReportItemStatus {
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

func tempestOwnerToAppOwner(owner appapi.Owner) *appv1.Owner {
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

func postTempestError(tempestClient *appapi.ClientWithResponses, taskID string, appErr error) error {
	errStr := appErr.Error()
	_, err := tempestClient.PostAppsOperationsReport(context.TODO(), appapi.PostAppsOperationsReportJSONRequestBody{
		TaskId:  taskID,
		Status:  appapi.ReportResponseStatusError,
		Message: &errStr,
	})
	return err
}
