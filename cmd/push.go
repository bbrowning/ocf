// The command flags, their descriptions, and some of the logic for
// merging manifests with flags come from Cloud Foundry's 'cf'
// tool. See the NOTICE file for more information.

package cmd

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/bbrowning/ocf/pkg/app"

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
	Image        string
}

type Manifest struct {
	Applications []app.Application `json:"applications"`
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

	cmd.Flags().StringVarP(&config.Buildpack, "buildpack", "b", "", "Custom buildpack by Git URL (e.g. 'https://github.com/cloudfoundry/java-buildpack.git') or Git URL with a branch or tag (e.g. 'https://github.com/cloudfoundry/java-buildpack.git#v3.3.0' for 'v3.3.0' tag). To use built-in buildpacks only, specify 'default' or 'null'")
	cmd.Flags().StringVarP(&config.Command, "command", "c", "", "Startup command, set to null to reset to default start command")
	cmd.Flags().StringVarP(&config.ManifestPath, "manifest-path", "f", "", "Path to manifest")
	// cmd.Flags().IntVarP(&config.Instances, "instances", "i", 1, "Number of instances")
	// cmd.Flags().StringVarP(&config.Disk, "disk", "k", "", "Disk limit (e.g. 256M, 1024M, 1G)")
	cmd.Flags().StringVarP(&config.Memory, "memory", "m", "", "Memory limit (e.g. 256M, 1024M, 1G)")
	cmd.Flags().StringVarP(&config.Path, "path", "p", "", "Path to app directory or to a zip file of the contents of the app directory")
	cmd.Flags().StringVarP(&config.Image, "image", "", "bbrowning/openshift-cloudfoundry-docker19", "Base Docker image to use when building and deploying applications")

	return cmd
}

func (config *PushConfig) Run(args []string) error {
	debugf("Config: %+v\n", config)

	manifestApps, err := config.getManifestApps()
	if err != nil {
		return err
	}
	debugf("manifestApps: %+v\n", manifestApps)

	flagsApp, err := config.getFlagsApp(args)
	if err != nil {
		return err
	}
	debugf("flagsApp: %+v\n", flagsApp)

	mergedApps, err := mergeAppsFromManifestAndFlags(manifestApps, flagsApp)
	if err != nil {
		return err
	}
	debugf("mergedApps: %+v\n", mergedApps)
	debugf("\n\n\n")

	for _, app := range mergedApps {
		if app.Name == "" {
			return errors.New("Error: no name found for app")
		}

		app.Push(config.Image)
	}

	return nil
}

func (config *PushConfig) getManifestApps() ([]app.Application, error) {
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
			return []app.Application{}, nil
		} else {
			return nil, err
		}
	}

	var m Manifest
	err = yaml.Unmarshal(y, &m)
	if err != nil {
		return nil, err
	}
	debugf("manifest: %+v\n", m)

	return m.Applications, nil
}

func (config *PushConfig) getFlagsApp(args []string) (app.Application, error) {
	app := app.Application{}

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

	if config.Memory != "" {
		mem := strings.TrimSuffix(strings.ToUpper(config.Memory), "B")
		matched, err := regexp.MatchString("^\\d+[EPTGMK]?$", mem)
		if err != nil {
			return app, err
		}
		if !matched {
			return app, errors.New("Memory string must be in the format of 8690K, 256M, 256MB, 1G, 1GB, etc")
		}
		app.Memory = mem
	}

	if config.Path != "" {
		app.Path = config.Path
	}

	return app, nil
}

func mergeAppsFromManifestAndFlags(manifestApps []app.Application, flagsApp app.Application) ([]app.Application, error) {
	var err error
	var apps []app.Application

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

func addApp(apps *[]app.Application, app app.Application) error {
	if app.Name == "" {
		return errors.New("App name is a required field")
	}

	app.SetupDefaults()

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

func debugf(format string, v ...interface{}) {
	if Debug {
		fmt.Printf(format, v...)
	}
}
