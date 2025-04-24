package cmd

import (
	"bufio"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
	"github.com/tempestdx/cli/internal/config"
)

//go:embed all:templates/*
var templatesFS embed.FS

var (
	appInitAppVersion string

	initCmd = &cobra.Command{
		Use:   "init <app_id> [flags]",
		Short: `Scaffold a "helloworld" Tempest App`,
		Args:  cobra.ExactArgs(1),
		RunE:  initRunE,
	}

	appIDRegex   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]+$`)
	versionRegex = regexp.MustCompile(`^v\d+$`)
)

func init() {
	appCmd.AddCommand(initCmd)

	initCmd.Flags().StringVarP(&appInitAppVersion, "version", "v", "v1", "The version of the app to initialize")
}

func initRunE(cmd *cobra.Command, args []string) error {
	appInitAppID := args[0]

	if !appIDRegex.MatchString(appInitAppID) {
		return fmt.Errorf("invalid App ID. Must be lowercase, and contain only letters, numbers, underscores, and dashes")
	}

	if !versionRegex.MatchString(appInitAppVersion) {
		return fmt.Errorf("invalid version format. Must be in the format 'v1', 'v2', etc")
	}

	workdir, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, cfgPath, err := config.ReadConfig()
	if err != nil {
		if errors.Is(err, config.ErrNoConfig) {
			// If there is no config already, create a new one in the
			// current directory.
			cmd.Println("There was no Tempest configuration found in this directory or above it.")
			cmd.Print("Initialize a new configuration here? ")
			yes := waitforYesNo()
			if !yes {
				cmd.Println("Exiting...")
				return nil
			}

			cfg = &config.TempestConfig{
				Version:  "v1",
				Apps:     make(map[string][]*config.AppVersion),
				BuildDir: ".build",
			}
			cfgPath = workdir
		} else {
			return err
		}
	}

	for appID, versions := range cfg.Apps {
		if appID == appInitAppID {
			for _, v := range versions {
				if v.Version == appInitAppVersion {
					return fmt.Errorf("app %s:%s already exists in the configuration", appID, appInitAppVersion)
				}
			}
		}
	}

	cfg.Apps[appInitAppID] = append(cfg.Apps[appInitAppID], &config.AppVersion{
		Path:    filepath.Join("apps", appInitAppID, appInitAppVersion),
		Version: appInitAppVersion,
	})

	fp := filepath.Join(cfgPath, "apps", appInitAppID, appInitAppVersion)

	cmd.Println(fmt.Sprintf(`Tempest App Initialization
--------------------------

Initializing app: %s:%s
Location: %s
`, appInitAppID, appInitAppVersion, fp))

	appFS, err := fs.Sub(templatesFS, "templates/helloworld")
	if err != nil {
		return err
	}

	var templates []string

	err = fs.WalkDir(appFS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || d.Type() == fs.ModeSymlink {
			return nil
		}

		templates = append(templates, path)
		return nil
	})
	if err != nil {
		return err
	}

	for _, f := range templates {
		t, err := template.ParseFS(appFS, f)
		if err != nil {
			return err
		}

		// TODO this should not do go.mod as part of this. Instead, it should do it in the main directory.

		// Remove the trailing underscore from the go files
		// 1. embed will not allow embedding files it believes are part of a separate module
		// 2. linting fails against these files
		// TODO: use a better templating system, or pull these templates from the examples repo
		f = strings.TrimSuffix(f, "_")

		dst := filepath.Join(fp, f)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}

		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		defer func() {
			if err := out.Close(); err != nil {
				// Log the error or handle it as needed
				fmt.Fprintf(os.Stderr, "error closing file: %v\n", err)
			}
		}()

		if err := t.Execute(out, struct {
			PackageName string
		}{
			PackageName: sanitizeAppID(appInitAppID),
		}); err != nil {
			return err
		}
	}

	// Create a go.mod in the cfgPath directory if it doesn't exist
	_, err = os.Stat(filepath.Join(cfgPath, "go.mod"))
	if err != nil {
		if os.IsNotExist(err) {
			err = exec.Command("go", "mod", "init", "apps").Run()
			if err != nil {
				return err
			}
		}
	}

	// Pull in the dependencies
	err = exec.Command("go", "mod", "tidy").Run()
	if err != nil {
		return err
	}

	// Generate the .build directory
	err = generateBuildDir(cfg, cfgPath, "", "")
	if err != nil {
		return err
	}

	// Generate the .gitignore file
	err = generateGitIgnore(cfgPath)
	if err != nil {
		return err
	}

	err = config.WriteConfig(cfg, cfgPath)
	if err != nil {
		return err
	}

	cmd.Printf(`
✅ App %s:%s initialized successfully!

Next steps:

1. To see the capabilities of your app, run:
   tempest app describe %s:%s
`, appInitAppID, appInitAppVersion, appInitAppID, appInitAppVersion)

	return nil
}

func generateGitIgnore(cfgPath string) error {
	gitignorePath := filepath.Join(cfgPath, ".gitignore")
	gitignoreContents := []byte("# Tempest build artifacts\n.build/\n")
	_, err := os.Stat(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			err := os.WriteFile(gitignorePath, gitignoreContents, 0o644)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	} else {
		// Read the existing .gitignore file
		contents, err := os.ReadFile(gitignorePath)
		if err != nil {
			return err
		}

		// Check if the .build/ line already exists
		scanner := bufio.NewScanner(strings.NewReader(string(contents)))
		for scanner.Scan() {
			if strings.TrimSpace(scanner.Text()) == ".build/" {
				return nil
			}
		}

		f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer func() {
			if err := f.Close(); err != nil {
				// Log the error or handle it as needed
				fmt.Fprintf(os.Stderr, "error closing file: %v\n", err)
			}
		}()

		_, err = f.Write(gitignoreContents)
		if err != nil {
			return err
		}
	}

	return nil
}

func generateBuildDir(cfg *config.TempestConfig, cfgPath, appID, version string) error {
	absBuildDir := filepath.Join(cfgPath, cfg.BuildDir)

	if err := os.MkdirAll(absBuildDir, 0o755); err != nil {
		return fmt.Errorf("create build directory: %w", err)
	}

	// Symlink cfgPath/apps/ to the cfg.BuildDir/apps/ directory
	err := os.Symlink(filepath.Join(cfgPath, "apps/"), filepath.Join(absBuildDir, "apps/"))
	if err != nil {
		if !os.IsExist(err) {
			return fmt.Errorf("symlink apps directory: %w", err)
		}
	}

	f, err := fs.ReadFile(templatesFS, "templates/build/main.go_")
	if err != nil {
		return fmt.Errorf("read main.go template: %w", err)
	}

	err = os.WriteFile(filepath.Join(absBuildDir, "main.go"), f, 0o644)
	if err != nil {
		return fmt.Errorf("write main.go: %w", err)
	}

	err = os.WriteFile(filepath.Join(absBuildDir, "apps.go"), appsDotGoContent(cfg, appID, version), 0o644)
	if err != nil {
		return fmt.Errorf("write apps.go: %w", err)
	}

	// Read the parent go.mod file
	parentModContent, err := os.ReadFile(filepath.Join(cfgPath, "go.mod"))
	hasParentMod := err == nil // Track if parent go.mod exists

	// Remove go.mod if it exists
	goModPath := filepath.Join(absBuildDir, "go.mod")
	if _, err := os.Stat(goModPath); err == nil {
		err := os.Remove(goModPath)
		if err != nil {
			return fmt.Errorf("remove existing go.mod: %w", err)
		}
	}

	// Remove go.sum if it exists
	goSumPath := filepath.Join(absBuildDir, "go.sum")
	if _, err := os.Stat(goSumPath); err == nil {
		err := os.Remove(goSumPath)
		if err != nil {
			return fmt.Errorf("remove existing go.sum: %w", err)
		}
	}

	modInit := exec.Command("go", "mod", "init", "tempestappserver")
	modInit.Dir = absBuildDir
	err = modInit.Run()
	if err != nil {
		return fmt.Errorf("go mod init: %w", err)
	}

	// Only attempt to copy dependencies if parent go.mod exists
	if hasParentMod {
		// Read the newly created go.mod to preserve module and go version
		newModContent, err := os.ReadFile(goModPath)
		if err != nil {
			return fmt.Errorf("read new go.mod: %w", err)
		}

		// Find and append everything after the first require from parent go.mod
		content := string(parentModContent)
		if idx := strings.Index(content, "require"); idx != -1 {
			remainingContent := string(newModContent) + "\n" + content[idx:]
			err = os.WriteFile(goModPath, []byte(remainingContent), 0600)
			if err != nil {
				return fmt.Errorf("write dependencies to go.mod: %w", err)
			}
		}
	}

	// Run go mod tidy
	modTidy := exec.Command("go", "mod", "tidy")
	modTidy.Dir = absBuildDir
	err = modTidy.Run()
	if err != nil {
		return fmt.Errorf("go mod tidy: %w", err)
	}

	return nil
}

func appsDotGoContent(cfg *config.TempestConfig, appID, version string) []byte {
	var av *config.AppVersion
	if appID != "" && version != "" {
		av = cfg.LookupAppByVersion(appID, version)
	}

	s := strings.Builder{}

	// Write the package and imports.
	s.WriteString("package main\n\n")
	if av == nil {
		// load and run all of the apps
		for appID, versions := range cfg.Apps {
			for _, version := range versions {
				s.WriteString(fmt.Sprintf("import %s \"%s\"\n", sanitizeAppID(appID)+version.Version, "tempestappserver/"+version.Path))
			}
		}
	} else {
		s.WriteString(fmt.Sprintf("import %s \"%s\"\n", sanitizeAppID(appID)+version, "tempestappserver/"+av.Path))
	}

	s.WriteString("\n")

	s.WriteString("func (s *AppServer) RegisterApps() {\n")
	if av == nil {
		for appID, versions := range cfg.Apps {
			for _, version := range versions {
				s.WriteString("\ts.apps = append(s.apps, &appHandler{\n")
				s.WriteString(fmt.Sprintf("\t\ta:   %s.App(),\n", sanitizeAppID(appID)+version.Version))
				s.WriteString(fmt.Sprintf("\t\tappID: \"%s\",\n", appID))
				s.WriteString(fmt.Sprintf("\t\tversion: \"%s\",\n", version.Version))
				s.WriteString("\t})\n")
			}
		}
	} else {
		s.WriteString("\ts.apps = append(s.apps, &appHandler{\n")
		s.WriteString(fmt.Sprintf("\t\ta:   %s.App(),\n", sanitizeAppID(appID)+version))
		s.WriteString(fmt.Sprintf("\t\tappID: \"%s\",\n", appID))
		s.WriteString(fmt.Sprintf("\t\tversion: \"%s\",\n", version))
		s.WriteString("\t})\n")
	}

	s.WriteString("}")

	return []byte(s.String())
}

var sanitizedAppID = regexp.MustCompile(`[^a-z0-9]+`)

// sanitizeAppID removes all non-alphabetic characters from the app ID.
func sanitizeAppID(appID string) string {
	sanitized := sanitizedAppID.ReplaceAllString(appID, "")

	return "app" + sanitized
}
