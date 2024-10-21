package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/config"
	"github.com/tempestdx/cli/internal/runner"
	"github.com/tempestdx/cli/internal/secret"
	appapi "github.com/tempestdx/openapi/app"
	appv1 "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1"
	appsdk "github.com/tempestdx/sdk-go/app"
	"github.com/zalando/go-keyring"
)

var connectCmd = &cobra.Command{
	Use:   "connect <app_id:app_version>",
	Short: "Connect your Tempest App to the Tempest API",
	Long: `The connect command is used to connect your Tempest App to the Tempest API.

This command will update the capabilities and schema of the App in Tempest, and allow you to serve the app.`,
	Args: cobra.ExactArgs(1),
	RunE: connectRunE,
}

func init() {
	appCmd.AddCommand(connectCmd)
}

func connectRunE(cmd *cobra.Command, args []string) error {
	id, version, err := splitAppVersion(args[0])
	if err != nil {
		return err
	}

	token := os.Getenv("TEMPEST_TOKEN")
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
		return fmt.Errorf("read config: %w", err)
	}

	appVersion := cfg.LookupAppByVersion(id, version)
	if appVersion == nil {
		return fmt.Errorf("app %s:%s not found", id, version)
	}

	err = generateBuildDir(cfg, cfgDir)
	if err != nil {
		return fmt.Errorf("generate build dir: %w", err)
	}

	runners, cancel, err := runner.StartApps(context.TODO(), cfg)
	if err != nil {
		return fmt.Errorf("start local app: %w", err)
	}
	defer cancel()

	var runner runner.Runner
	for _, r := range runners {
		if r.AppID == id && r.Version == version {
			runner = r
			break
		}
	}

	cmd.Println(`Tempest App Connect
-----------------------`)

	res, err := runner.Client.Describe(context.TODO(), connect.NewRequest(&appv1.DescribeRequest{}))
	if err != nil {
		return fmt.Errorf("reach private app: %w", err)
	}

	cmd.Println(formatDescribeResponse(res.Msg, id, appVersion))

	cmd.Printf("The above capabilities will be connected to app %s at version %s.\n\n", id, version)
	cmd.Print("Continue to connect this app to the Tempest API? ")
	yes := waitforYesNo()
	if !yes {
		cmd.Println("Exiting...")
		return nil
	}

	cmd.Println()

	waveClient, err := appapi.NewClientWithResponses(
		apiEndpoint,
		appapi.WithHTTPClient(&http.Client{
			Timeout:   10 * time.Second,
			Transport: secret.NewTransportWithToken(token),
		}),
	)
	if err != nil {
		return fmt.Errorf("connect to API: %w", err)
	}

	var resources []appapi.ResourceDefinition
	for _, r := range res.Msg.GetResourceDefinitions() {
		propertySchema := r.PropertiesSchema.AsMap()
		instructionsMarkdown := r.InstructionsMarkdown

		def := appapi.ResourceDefinition{
			Type:                 r.Type,
			DisplayName:          r.DisplayName,
			Description:          r.Description,
			PropertyJsonSchema:   &propertySchema,
			LifecycleStage:       appsdk.LifecycleStage(r.LifecycleStage).String(),
			CreateSupported:      r.CreateSupported,
			ReadSupported:        r.ReadSupported,
			UpdateSupported:      r.UpdateSupported,
			DeleteSupported:      r.DeleteSupported,
			ListSupported:        r.ListSupported,
			InstructionsMarkdown: &instructionsMarkdown,
			HealthcheckSupported: r.HealthcheckSupported,
		}

		if r.CreateSupported {
			m := r.CreateInputSchema.AsMap()
			def.CreateInputSchema = &m
		}

		if r.UpdateSupported {
			m := r.UpdateInputSchema.AsMap()
			def.UpdateInputSchema = &m
		}

		if len(r.Links) > 0 {
			items := make([]appapi.LinksItem, 0, len(r.Links))
			for _, link := range r.Links {
				items = append(items, appapi.LinksItem{
					Title: link.Title,
					Url:   link.Url,
					Type:  appapi.LinksItemType(appsdk.LinkType(link.Type).String()),
				})
			}

			links := &appapi.Links{
				Links: &items,
			}

			def.Links = links
		}

		resources = append(resources, def)
	}

	apiRes, err := waveClient.PostAppsVersionConnectWithResponse(context.TODO(), appapi.PostAppsVersionConnectJSONRequestBody{
		AppId:     id,
		Version:   version,
		Resources: resources,
	})
	if err != nil {
		return fmt.Errorf("connect version: %w", err)
	}

	if apiRes.HTTPResponse.StatusCode != http.StatusOK {
		return fmt.Errorf("connect version: %d %s", apiRes.HTTPResponse.StatusCode, string(apiRes.Body))
	}

	if apiRes.JSON200.Metadata != nil && apiRes.JSON200.Metadata.TempestAppUrl != nil {
		cmd.Println("Successfully connected app to Tempest API. 🎉 View details at " + *apiRes.JSON200.Metadata.TempestAppUrl + ".\n")
	} else {
		cmd.Println("Successfully connected app to Tempest API. 🎉 View details at https://app.tempestdx.com.")
	}

	cmd.Println("To serve your app, run:")
	cmd.Println("\ttempest app serve " + id + ":" + version)

	return nil
}

func waitforYesNo() bool {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("(y/n): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		switch input {
		case "y", "Y":
			return true
		case "n", "N":
			return false
		default:
			fmt.Println("Invalid input. Please enter 'y' or 'n'.")
		}
	}
}
