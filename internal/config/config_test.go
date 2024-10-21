package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tempestdx/cli/internal/config"
)

var testContent = []byte(`version: v1
apps:
  app1:
    - path: /path/to/app1/v1
      version: v1
  app2:
    - path: /path/to/app2/v2
      version: v2
build_dir: /path/to/.build
`)

func TestReadConfigSuccess(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "test-tempest-config")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(tempDir)) }()

	// On macOS, the temp directory is a symlink, so we need to evaluate it
	// https://github.com/golang/go/issues/56259
	tempDir, err = filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	tempestFilePath := filepath.Join(tempDir, "tempest.yaml")
	err = os.WriteFile(tempestFilePath, testContent, 0o644)
	require.NoError(t, err)

	// Temporarily change the working directory to the tempDir
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer func() { require.NoError(t, os.Chdir(originalDir)) }()

	err = os.Chdir(tempDir)
	require.NoError(t, err)

	cfg, dir, err := config.ReadConfig()
	require.NoError(t, err)

	assert.Equal(t, tempDir, dir)
	assert.Equal(t, "/path/to/.build", cfg.BuildDir)
	assert.Len(t, cfg.Apps, 2)
	assert.Equal(t, []*config.AppVersion{
		{
			Path:    "/path/to/app1/v1",
			Version: "v1",
		},
	}, cfg.Apps["app1"])
	assert.Equal(t, []*config.AppVersion{
		{
			Path:    "/path/to/app2/v2",
			Version: "v2",
		},
	}, cfg.Apps["app2"])
}

func TestWriteConfigSuccess(t *testing.T) {
	// Create a temporary directory for the test
	tempDir, err := os.MkdirTemp("", "test-tempest-config")
	require.NoError(t, err)
	defer func() { require.NoError(t, os.RemoveAll(tempDir)) }() // Clean up after test

	// On macOS, the temp directory is a symlink, so we need to evaluate it
	// https://github.com/golang/go/issues/56259
	tempDir, err = filepath.EvalSymlinks(tempDir)
	require.NoError(t, err)

	// Create a TempestConfig structure to write
	cfg := &config.TempestConfig{
		BuildDir: "/path/to/.build",
		Apps: map[string][]*config.AppVersion{
			"app1": {
				{
					Path:    "/path/to/app1/v1",
					Version: "v1",
				},
			},
			"app2": {
				{
					Path:    "/path/to/app2/v2",
					Version: "v2",
				},
			},
		},
	}

	err = config.WriteConfig(cfg, tempDir)
	require.NoError(t, err)

	// Verify the written file exists
	writtenFilePath := filepath.Join(tempDir, "tempest.yaml")
	_, err = os.Stat(writtenFilePath)
	require.NoError(t, err)

	writtenContent, err := os.ReadFile(writtenFilePath)
	require.NoError(t, err)

	require.Equal(t, string(testContent), string(writtenContent))
}
