// The command flags, their descriptions, and some of the logic for
// merging manifests with flags come from Cloud Foundry's 'cf'
// tool. See the NOTICE file for more information.

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ghodss/yaml"
	"github.com/imdario/mergo"
	"github.com/spf13/cobra"
)

const (
	pushCmdLong = `
Create a new application or update an existing one.

This command emulates Cloud Foundry's 'cf push' command but targeting
OpenShift instead. Not all the Cloud Foundry options are supported;
those that are supported are documented in the usage information
below.`

	pushCmdExample = `
  # Create a new application from a local Java artifact
  %[1]s push target/foo.jar

  # Create a new application named my-new-app from the code in the current directory
  %[1]s push my-new-app

  # Create a new application from a manifest.yml
  %[1]s push

  # Update an existing application with a manifest.yml
  %[1]s push`
)

// PushConfig contains all the necessary configuration for the push command
type PushConfig struct {
	Buildpack    string
	Command      string
	ManifestPath string
	Instances    int
	Disk         string
	Memory       string
	Path         string
}

type Manifest struct {
	Applications []Application `json:"applications"`
}

type Application struct {
	Name      string `json:"name"`
	Buildpack string `json:"buildpack"`
	Command   string `json:"command"`
	DiskQuota string `json:"disk_quota"`
	Instances int    `json:"instances"`
	Memory    string `json:"memory"`
	Path      string `json:"path"`
}

func init() {
	RootCmd.AddCommand(newPushCmd("ocf"))
}

func newPushCmd(commandName string) *cobra.Command {
	config := &PushConfig{}
	cmd := &cobra.Command{
		Use:     "push",
		Short:   "Create a new application or update an existing one.",
		Long:    pushCmdLong,
		Example: fmt.Sprintf(pushCmdExample, commandName),
		Run: func(cmd *cobra.Command, args []string) {
			err := config.Run(args)
			if err != nil {
				fmt.Printf("err: %v\n", err)
			}
		},
	}

	cmd.Flags().StringVarP(&config.Buildpack, "buildpack", "b", "", "Custom buildpack by name (e.g. my-buildpack) or Git URL (e.g. 'https://github.com/cloudfoundry/java-buildpack.git') or Git URL with a branch or tag (e.g. 'https://github.com/cloudfoundry/java-buildpack.git#v3.3.0' for 'v3.3.0' tag). To use built-in buildpacks only, specify 'default' or 'null'")
	cmd.Flags().StringVarP(&config.Command, "command", "c", "", "Startup command, set to null to reset to default start command")
	cmd.Flags().StringVarP(&config.ManifestPath, "manifest-path", "f", "", "Path to manifest")
	cmd.Flags().IntVarP(&config.Instances, "instances", "i", 1, "Number of instances")
	cmd.Flags().StringVarP(&config.Disk, "disk", "k", "", "Disk limit (e.g. 256M, 1024M, 1G)")
	cmd.Flags().StringVarP(&config.Memory, "memory", "m", "", "Memory limit (e.g. 256M, 1024M, 1G)")
	cmd.Flags().StringVarP(&config.Path, "path", "p", "", "Path to app directory or to a zip file of the contents of the app directory")

	return cmd
}

func (config *PushConfig) Run(args []string) error {
	fmt.Printf("Config: %+v\n", config)

	manifestApps, err := config.getManifestApps()
	if err != nil {
		return err
	}
	fmt.Printf("manifestApps: %+v\n", manifestApps)

	flagsApp, err := config.getFlagsApp(args)
	if err != nil {
		return err
	}
	fmt.Printf("flagsApp: %+v\n", flagsApp)

	mergedApps, err := config.mergeAppsFromManifestAndFlags(manifestApps, flagsApp)
	if err != nil {
		return err
	}
	fmt.Printf("mergedApps: %+v\n", mergedApps)

	fmt.Println("")
	fmt.Println("")
	fmt.Println("")
	for _, app := range mergedApps {
		if app.Name == "" {
			return errors.New("Error: no name found for app")
		}

		// TODO: help user select the correct project instead of just
		// assuming they've already done that
		err := exec.Command("oc", "whoami").Run()
		if err != nil {
			loginCmd := exec.Command("oc", "login")
			loginCmd.Stdin = os.Stdin
			loginCmd.Stdout = os.Stdout
			loginCmd.Stderr = os.Stderr
			err = loginCmd.Run()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}

		output, err := exec.Command("oc", "project").CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}

		// Ensure the build exists
		output, err = exec.Command("oc", "get", "bc", app.Name).CombinedOutput()
		if strings.Contains(string(output), "not found") {
			newCmd := exec.Command("oc", "new-build", "bbrowning/openshift-cloudfoundry@sha256:bbdf7d8624b0a03230fa8370d16057546986c7afa4fcb22d71e7691e4a96c3cb",
				"--binary=true", fmt.Sprint("--name=", app.Name))
			fmt.Printf("Creating build with command: %s\n", strings.Join(newCmd.Args, " "))
			output, err = newCmd.CombinedOutput()
			fmt.Println(string(output))
			// if err != nil {
			// 	fmt.Fprintln(os.Stderr, err)
			// 	os.Exit(1)
			// }
		} else if err != nil {
			fmt.Println(string(output))
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else {
			fmt.Printf("Build configuration already exists for %s, skipping creating one\n", app.Name)
		}

		// Start the build
		var pathArg string
		if fi, err := os.Stat(app.Path); err != nil || fi.IsDir() {
			pathArg = fmt.Sprint("--from-dir=", app.Path)
		} else {
			pathArg = fmt.Sprint("--from-file=", app.Path)
		}
		startBuildCmd := exec.Command("oc", "start-build", app.Name, pathArg, "--follow")
		startBuildCmd.Stdout = os.Stdout
		startBuildCmd.Stderr = os.Stderr
		// startBuildCmd.Env = append(os.Environ(), "DEBUG=true")
		fmt.Printf("Starting build with command: %s\n", strings.Join(startBuildCmd.Args, " "))
		err = startBuildCmd.Run()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}

	return nil
}

func (config *PushConfig) getManifestApps() ([]Application, error) {
	var path string
	var err error
	if config.ManifestPath != "" {
		path = config.ManifestPath
	} else {
		path, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	if info, err := os.Stat(path); err == nil && info.IsDir() {
		path = filepath.Join(path, "manifest.yml")
	}
	y, err := ioutil.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Application{}, nil
		} else {
			return nil, err
		}
	}

	var m Manifest
	err = yaml.Unmarshal(y, &m)
	if err != nil {
		return nil, err
	}
	fmt.Printf("manifest: %+v\n", m)

	return m.Applications, nil
}

func (config *PushConfig) getFlagsApp(args []string) (Application, error) {
	app := Application{}

	if len(args) > 0 {
		app.Name = args[0]
	}

	if config.Buildpack != "" && config.Buildpack != "null" && config.Buildpack != "default" {
		app.Buildpack = config.Buildpack
	}

	if config.Command != "" && config.Command != "null" && config.Command != "default" {
		app.Command = config.Command
	}

	if config.Instances > 0 {
		app.Instances = config.Instances
	}

	if config.Path != "" {
		app.Path = config.Path
	}

	return app, nil
}

func (config *PushConfig) mergeAppsFromManifestAndFlags(manifestApps []Application, flagsApp Application) ([]Application, error) {
	var err error
	var apps []Application

	switch len(manifestApps) {
	case 0:
		if flagsApp.Name == "" {
			return nil, errors.New("Manifest file is not found in the current directory, please provide either an app name or manifest")
		}
		err = addApp(&apps, flagsApp)
	case 1:
		mergo.MergeWithOverwrite(&manifestApps[0], flagsApp)
		err = addApp(&apps, manifestApps[0])
	default:
		selectedAppName := flagsApp.Name

		// TODO: Check for flags and multiple apps error condition

		if selectedAppName != "" {
			var foundApp bool
			for _, currentApp := range manifestApps {
				if currentApp.Name == selectedAppName {
					foundApp = true
					err = addApp(&apps, currentApp)
				}
			}
			if !foundApp {
				err = errors.New(fmt.Sprintf("Could not find app named %s in manifest", selectedAppName))
			}
		} else {
			for _, manifestApp := range manifestApps {
				err = addApp(&apps, manifestApp)
			}
		}
	}

	if err != nil {
		return nil, err
	}

	return apps, nil
}

func addApp(apps *[]Application, app Application) error {
	if app.Name == "" {
		return errors.New("App name is a required field")
	}

	if app.Path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		app.Path = cwd
	}

	*apps = append(*apps, app)
	return nil
}
