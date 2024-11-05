package runner

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"connectrpc.com/connect"
	"github.com/cenkalti/backoff/v4"
	"github.com/tempestdx/cli/internal/config"
	appv1 "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1"
	appv1connect "github.com/tempestdx/protobuf/gen/go/tempestdx/app/v1/appv1connect"
)

type Runner struct {
	Client  appv1connect.AppServiceClient
	Path    string
	AppID   string
	Version string
}

// Start the app runner for all apps and return clients for each service.
func StartApps(ctx context.Context, cfg *config.TempestConfig, cfgDir string) ([]Runner, func(), error) {
	absBuildDir := filepath.Join(cfgDir, cfg.BuildDir)

	var cmd *exec.Cmd
	info, err := os.Stat(absBuildDir)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		cmd = exec.Command("go", "run", ".")
		cmd.Dir = absBuildDir
	} else {
		return nil, nil, fmt.Errorf("invalid build directory: %s", absBuildDir)
	}

	// Start process
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}
	err = cmd.Start()
	if err != nil {
		return nil, nil, err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Println("App logged to stderr", "line", scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		return nil, nil, fmt.Errorf("scan: %w", scanner.Err())
	}

	port := scanner.Text()

	go func() {
		for scanner.Scan() {
			fmt.Println("App logged to stdout", "line", scanner.Text())
		}
	}()

	var runners []Runner
	for appID, versions := range cfg.Apps {
		for _, version := range versions {
			runner, err := createRunner(ctx, appID, version, port)
			if err != nil {
				return nil, nil, err
			}

			runners = append(runners, runner)
		}
	}

	cancel := func() {
		err = cmd.Process.Kill()
		if err != nil {
			fmt.Println("failed to kill app", "error", err)
		}
	}

	return runners, cancel, nil
}

// StartApp starts a single app runner and returns a client for the service.
func StartApp(ctx context.Context, cfg *config.TempestConfig, cfgDir, appID, version string) (Runner, func(), error) {
	absBuildDir := filepath.Join(cfgDir, cfg.BuildDir)

	var cmd *exec.Cmd
	info, err := os.Stat(absBuildDir)
	if err != nil {
		return Runner{}, nil, err
	}
	if info.IsDir() {
		cmd = exec.Command("go", "run", ".")
		cmd.Dir = absBuildDir
	} else {
		return Runner{}, nil, fmt.Errorf("invalid build directory: %s", absBuildDir)
	}

	// Start process
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Runner{}, nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return Runner{}, nil, err
	}
	err = cmd.Start()
	if err != nil {
		return Runner{}, nil, err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			fmt.Println("App logged to stderr", "line", scanner.Text())
		}
	}()

	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() {
		return Runner{}, nil, fmt.Errorf("scan: %w", scanner.Err())
	}

	port := scanner.Text()

	go func() {
		for scanner.Scan() {
			fmt.Println("App logged to stdout", "line", scanner.Text())
		}
	}()

	av := cfg.LookupAppByVersion(appID, version)

	runner, err := createRunner(ctx, appID, av, port)
	if err != nil {
		return Runner{}, nil, err
	}

	cancel := func() {
		err = cmd.Process.Kill()
		if err != nil {
			fmt.Println("failed to kill app", "error", err)
		}
	}

	return runner, cancel, nil
}

func createRunner(ctx context.Context, appID string, version *config.AppVersion, port string) (Runner, error) {
	path := appID + "-" + version.Version
	client := appv1connect.NewAppServiceClient(http.DefaultClient, fmt.Sprintf("http://localhost:%s/%s", port, path))

	// Confirm plugin is reachable.
	err := backoff.Retry(func() error {
		_, err := client.Describe(ctx, connect.NewRequest(&appv1.DescribeRequest{}))
		return err
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(time.Second), 5))
	if err != nil {
		return Runner{}, err
	}

	return Runner{
		Client:  client,
		Path:    path,
		AppID:   appID,
		Version: version.Version,
	}, nil
}
