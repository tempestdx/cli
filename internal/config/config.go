package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const tempestYAMLName = "tempest.yaml"

var ErrNoConfig = errors.New("no tempest.yaml found")

type TempestConfig struct {
	// The version of the config file. Will default to "v1" if not set.
	Version  string                   `yaml:"version"`
	Apps     map[string][]*AppVersion `yaml:"apps"`
	BuildDir string                   `yaml:"build_dir"`
}

type AppVersion struct {
	// Full Path to the app code.
	Path string `yaml:"path"`
	// The version of the app.
	Version string `yaml:"version"`
}

// ReadConfig reads the tempest.yaml file in the current directory or any parent
// directory. It returns the directory it found the file in, the config, and an
// error if one occurred.
func ReadConfig() (*TempestConfig, string, error) {
	dir, err := findFile(tempestYAMLName)
	if err != nil {
		return nil, "", err
	}

	f, err := os.Open(filepath.Join(dir, tempestYAMLName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", ErrNoConfig
		}
		return nil, "", err
	}

	decoder := yaml.NewDecoder(f)
	decoder.KnownFields(true)

	var cfg TempestConfig
	err = decoder.Decode(&cfg)
	if err != nil {
		return nil, "", err
	}

	if cfg.Version == "" {
		cfg.Version = "v1"
	}

	return &cfg, dir, nil
}

func WriteConfig(cfg *TempestConfig, dir string) error {
	f, err := os.Create(filepath.Join(dir, tempestYAMLName))
	if err != nil {
		return err
	}
	defer func() {
		err := f.Close()
		if err != nil {
			panic(err)
		}
	}()

	if cfg.Version == "" {
		cfg.Version = "v1"
	}

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)

	err = encoder.Encode(cfg)
	if err != nil {
		return err
	}

	return nil
}

// Find the file with fileName in the current directory or any parent directory.
func findFile(fileName string) (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		filePath := filepath.Join(currentDir, fileName)
		if _, err := os.Stat(filePath); err == nil {
			return currentDir, nil
		}

		if currentDir == "/" {
			break
		}

		currentDir = filepath.Dir(currentDir)
	}

	return "", nil
}

func (c *TempestConfig) LookupAppByVersion(appID, version string) *AppVersion {
	if c.Apps == nil {
		return nil
	}

	if _, ok := c.Apps[appID]; !ok {
		return nil
	}

	for _, v := range c.Apps[appID] {
		if v.Version == version {
			return v
		}
	}

	return nil
}
