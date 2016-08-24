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
	"regexp"
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
	Image        string
}

type Manifest struct {
	Applications []Application `json:"applications"`
}

type Application struct {
	Name      string   `json:"name"`
	Buildpack string   `json:"buildpack"`
	Command   string   `json:"command"`
	DiskQuota string   `json:"disk_quota"`
	Instances int      `json:"instances"`
	Memory    string   `json:"memory"`
	Path      string   `json:"path"`
	Services  []string `json:"services"`
	execer    Execer
}

type ExecCmd interface {
	Run() error
	CombinedOutput() ([]byte, error)
	AttachStdIO()
	ArgsString() string
}

type DefaultCmd struct {
	*exec.Cmd
}

type Execer interface {
	Oc(args ...string) ExecCmd
}

type DefaultExecer struct {
}

func (cmd *DefaultCmd) AttachStdIO() {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func (cmd *DefaultCmd) ArgsString() string {
	return strings.Join(cmd.Args, " ")
}

func (execer *DefaultExecer) Oc(args ...string) ExecCmd {
	return &DefaultCmd{exec.Command("oc", args...)}
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

		app.ensureLoggedIn()
		// TODO: help user select the correct project instead of just
		// assuming they've already done that
		app.displayProject()
		app.ensureBuildExists(config.Image)
		app.startBuild()
		app.ensureDeploymentExists()
		app.ensureServiceExists()
		app.ensureRouteExists()
		app.displayRoute()
	}

	return nil
}

func (app *Application) ensureLoggedIn() {
	err := app.execer.Oc("whoami").Run()
	if err != nil {
		loginCmd := app.execer.Oc("login")
		loginCmd.AttachStdIO()
		err = loginCmd.Run()
		if err != nil {
			exitWithError(err)
		}
	}
}

func (app *Application) displayProject() {
	output, err := app.execer.Oc("project").CombinedOutput()
	fmt.Println(string(output))
	if err != nil {
		exitWithError(err)
	}
}

func (app *Application) ensureBuildExists(image string) {
	output, err := app.execer.Oc("get", "bc", app.Name).CombinedOutput()
	if strings.Contains(string(output), "not found") {
		newCmd := app.execer.Oc(app.createBuildArgs(image)...)
		fmt.Printf("==> Creating build with command: %s\n", newCmd.ArgsString())
		// oc new-build sometimes gives a non-zero exit status for ignorable errors
		output, _ = newCmd.CombinedOutput()
		fmt.Println(string(output))
	} else if err != nil {
		exitWithOutputAndError(output, err)
	} else {
		fmt.Printf("==> Build configuration already exists for %s, skipping creating one\n", app.Name)
	}
}

func (app *Application) createBuildArgs(image string) []string {
	var buildpack string
	if app.Buildpack != "" {
		buildpack = fmt.Sprint("BUILDPACK_URL=", app.Buildpack)
	} else {
		buildpack = ""
	}
	return []string{"new-build", image, "--binary=true",
		fmt.Sprint("--name=", app.Name), buildpack}
}

func (app *Application) startBuild() {
	var pathArg string
	if fi, err := os.Stat(app.Path); err != nil || fi.IsDir() {
		pathArg = fmt.Sprint("--from-dir=", app.Path)
	} else {
		pathArg = fmt.Sprint("--from-file=", app.Path)
	}
	startBuildCmd := app.execer.Oc("start-build", app.Name, pathArg, "--follow")
	startBuildCmd.AttachStdIO()
	fmt.Printf("==> Starting build with command: %s\n", startBuildCmd.ArgsString())
	err := startBuildCmd.Run()
	if err != nil {
		exitWithError(err)
	}
}

func (app *Application) ensureDeploymentExists() {
	output, err := app.execer.Oc("get", "dc", app.Name).CombinedOutput()
	if strings.Contains(string(output), "not found") {
		repoAndImage, err := app.execer.Oc("get", "is", app.Name, "-o", "template", "--template={{.status.dockerImageRepository}}").CombinedOutput()
		if err != nil {
			exitWithOutputAndError(repoAndImage, err)
		}
		env, err := app.envForServices()
		if err != nil {
			exitWithError(err)
		}
		newCmd := app.execer.Oc(app.createDeploymentArgs(string(repoAndImage), env)...)
		fmt.Printf("==> Creating deployment config with command: %s\n", newCmd.ArgsString())
		output, err = newCmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			exitWithError(err)
		}
	} else if err != nil {
		exitWithOutputAndError(output, err)
	} else {
		fmt.Printf("==> Deployment config already exists for %s, redeploying\n", app.Name)
		output, err = app.execer.Oc("deploy", app.Name, "--latest").CombinedOutput()
		if err != nil {
			exitWithOutputAndError(output, err)
		}
	}
}

func (app *Application) envForServices() ([]string, error) {
	var env []string
	var serviceNames []string
	if len(app.Services) > 0 {
		for _, service := range app.Services {
			envPrefix := strings.ToUpper(strings.Replace(service, "-", "_", -1))
			serviceNames = append(serviceNames, envPrefix)
			output, err := app.execer.Oc("env", "dc", service, "--list").CombinedOutput()
			if err != nil {
				return env, errors.New(fmt.Sprintf("Error: Bound service %s not found\n", service))
			}
			var label string
			for _, line := range strings.Split(string(output), "\n") {
				switch {
				case strings.HasPrefix(line, "POSTGRESQL"):
					label = "postgresql"
				case strings.HasPrefix(line, "MYSQL"):
					label = "mysql"
				case strings.HasPrefix(line, "MONGODB"):
					label = "mongodb"
				}
				switch {
				case strings.Contains(line, "_USER="):
					addServiceEnv(&env, envPrefix, "_USER", line)
				case strings.Contains(line, "_PASSWORD="):
					addServiceEnv(&env, envPrefix, "_PASSWORD", line)
				case strings.Contains(line, "_DATABASE="):
					addServiceEnv(&env, envPrefix, "_DATABASE", line)
				}
			}
			env = append(env, fmt.Sprint(envPrefix, "_LABEL=", label, ""))
		}
		env = append(env, fmt.Sprint("CF_BOUND_SERVICES=", strings.Join(serviceNames, " ")))
	}
	return env, nil
}

func addServiceEnv(env *[]string, prefix string, suffix string, line string) {
	val := strings.Split(line, "=")[1]
	*env = append(*env, fmt.Sprint(prefix, suffix, "=", val))
}

func (app *Application) createDeploymentArgs(repoAndImage string, env []string) []string {
	var limits string
	if app.Memory != "" {
		limits = fmt.Sprint("--limits=memory=", app.Memory)
		env = append(env, fmt.Sprint("MEMORY_LIMIT=", app.Memory))
	} else {
		limits = ""
	}
	if app.Command != "" {
		env = append(env, fmt.Sprint("CF_COMMAND=", app.Command))
	}
	envStr := fmt.Sprint("--env=", strings.Join(env, ","))
	return []string{"run", app.Name, fmt.Sprint("--image=", repoAndImage),
		limits, envStr}
}

func (app *Application) ensureServiceExists() {
	output, err := app.execer.Oc("get", "svc", app.Name).CombinedOutput()
	if strings.Contains(string(output), "not found") {
		newCmd := app.execer.Oc("expose", "dc", app.Name, "--port=8080")
		fmt.Printf("==> Creating service with command: %s\n", newCmd.ArgsString())
		output, err = newCmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			exitWithError(err)
		}
	} else if err != nil {
		exitWithOutputAndError(output, err)
	} else {
		fmt.Printf("==> Service already exists for %s, skipping creating one\n", app.Name)
	}
}

func (app *Application) ensureRouteExists() {
	output, err := app.execer.Oc("get", "route", app.Name).CombinedOutput()
	if strings.Contains(string(output), "not found") {
		newCmd := app.execer.Oc("expose", "svc", app.Name)
		fmt.Printf("==> Creating route with command: %s\n", newCmd.ArgsString())
		output, err = newCmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			exitWithError(err)
		}
	} else if err != nil {
		exitWithOutputAndError(output, err)
	} else {
		fmt.Printf("==> Route already exists for %s, skipping creating one\n", app.Name)
	}
}

func (app *Application) displayRoute() {
	output, err := app.execer.Oc("get", "route", app.Name, "-o", "template",
		"--template={{.spec.host}}").CombinedOutput()
	if err != nil {
		exitWithOutputAndError(output, err)
	} else {
		fmt.Printf("==> Your application is available at %s\n", output)
	}
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
	debugf("manifest: %+v\n", m)

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

func mergeAppsFromManifestAndFlags(manifestApps []Application, flagsApp Application) ([]Application, error) {
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

	if app.execer == nil {
		app.execer = new(DefaultExecer)
	}

	*apps = append(*apps, app)
	return nil
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func exitWithOutputAndError(output []byte, err error) {
	fmt.Println(string(output))
	exitWithError(err)
}

func debugf(format string, v ...interface{}) {
	if Debug {
		fmt.Printf(format, v...)
	}
}
