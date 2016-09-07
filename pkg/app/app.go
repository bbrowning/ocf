package app

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/bbrowning/ocf/pkg/oc"
)

type Application struct {
	Name      string   `json:"name"`
	Buildpack string   `json:"buildpack"`
	Command   string   `json:"command"`
	DiskQuota string   `json:"disk_quota"`
	Instances int      `json:"instances"`
	Memory    string   `json:"memory"`
	Path      string   `json:"path"`
	Services  []string `json:"services"`
	oc        oc.Oc
}

const BoundServices string = "CF_BOUND_SERVICES"
const BuildpackUrl string = "BUILDPACK_URL"

func (app *Application) Push(image string) {
	app.setupDefaults()
	app.ensureLoggedIn()
	// TODO: help user select the correct project instead of just
	// assuming they've already done that
	app.displayProject()
	app.ensureBuildExists(image)
	app.startBuild()
	app.ensureDeploymentExists()
	app.ensureServiceExists()
	app.ensureRouteExists()
	app.displayRoute()
}

func (app *Application) BindService(service string) error {
	app.setupDefaults()
	app.ensureLoggedIn()
	app.displayProject()

	appExists, err := app.deploymentExists()
	if err != nil {
		return err
	}
	if !appExists {
		return errors.New(fmt.Sprintf("Error: Application %s not found\n", app.Name))
	}

	envPrefix := envPrefixFromService(service)
	env, err := app.envForServiceBinding(service, envPrefix)
	if err != nil {
		return err
	}

	appEnv, err := app.oc.Env("dc", app.Name)
	if err != nil {
		return err
	}

	boundServices := appEnv[BoundServices]
	alreadyBound, err := regexp.MatchString(fmt.Sprint("\\s?", envPrefix, "\\s?"), boundServices)
	if alreadyBound {
		return errors.New(fmt.Sprintf("Error: Service %s already bound to application %s\n", service, app.Name))
	}
	boundServices = strings.TrimLeft(fmt.Sprint(boundServices, " ", envPrefix), " ")

	env[BoundServices] = boundServices

	err = app.oc.SetEnv("dc", app.Name, env)
	if err != nil {
		return err
	}

	return nil
}

func (app *Application) UnbindService(service string) error {
	app.setupDefaults()
	app.ensureLoggedIn()
	app.displayProject()

	appExists, err := app.deploymentExists()
	if err != nil {
		return err
	}
	if !appExists {
		return errors.New(fmt.Sprintf("Error: Application %s not found\n", app.Name))
	}

	envPrefix := envPrefixFromService(service)
	appEnv, err := app.oc.Env("dc", app.Name)
	if err != nil {
		return err
	}

	newEnv := make(map[string]string)

	for key, _ := range appEnv {
		if strings.HasPrefix(key, envPrefix) {
			newEnv[key] = "-"
		}
	}

	if strings.Contains(appEnv[BoundServices], envPrefix) {
		newEnv[BoundServices] = strings.Trim(
			strings.Replace(appEnv[BoundServices], envPrefix, "", -1), " ")

		err = app.oc.SetEnv("dc", app.Name, newEnv)
		if err != nil {
			return err
		}
	} else {
		return errors.New(fmt.Sprintf("Error: Service %s not bound to application %s\n", service, app.Name))
	}

	return nil
}

func (app *Application) setupDefaults() {
	if app.oc == nil {
		app.oc = new(oc.DefaultOc)
	}
}

func (app *Application) ensureLoggedIn() {
	loggedIn := app.oc.LoggedIn()
	if !loggedIn {
		loginCmd := app.oc.Exec("login")
		loginCmd.AttachStdIO()
		err := loginCmd.Run()
		if err != nil {
			exitWithError(err)
		}
	}
}

func (app *Application) displayProject() error {
	project, err := app.oc.Project()
	fmt.Printf("Using project %s\n", project)
	return err
}

func (app *Application) ensureBuildExists(image string) {
	exists, err := app.oc.Exists("bc", app.Name)
	if err != nil {
		exitWithError(err)
	} else if !exists {
		env := make(map[string]string)
		if app.Buildpack != "" {
			env[BuildpackUrl] = app.Buildpack
		}
		app.oc.NewBuild(image, app.Name, env)
	} else {
		fmt.Printf("==> Build configuration already exists for %s, updating\n", app.Name)
		buildEnv, err := app.oc.Env("bc", app.Name)
		if err != nil {
			exitWithError(err)
		}
		if app.Buildpack != buildEnv[BuildpackUrl] {
			app.oc.SetEnv("bc", app.Name, map[string]string{BuildpackUrl: app.Buildpack})
		}
	}
}

func (app *Application) startBuild() {
	var pathArg string
	if fi, err := os.Stat(app.Path); err != nil || fi.IsDir() {
		pathArg = fmt.Sprint("--from-dir=", app.Path)
	} else {
		pathArg = fmt.Sprint("--from-file=", app.Path)
	}
	startBuildCmd := app.oc.Exec("start-build", app.Name, pathArg, "--follow")
	startBuildCmd.AttachStdIO()
	fmt.Printf("==> Starting build with command: %s\n", startBuildCmd.ArgsString())
	err := startBuildCmd.Run()
	if err != nil {
		exitWithError(err)
	}
}

func (app *Application) deploymentExists() (bool, error) {
	return app.oc.Exists("dc", app.Name)
}

func (app *Application) ensureDeploymentExists() {
	exists, err := app.deploymentExists()
	if err != nil {
		exitWithError(err)
	}
	if !exists {
		repoAndImage, err := app.oc.Exec("get", "is", app.Name, "-o", "template", "--template={{.status.dockerImageRepository}}").CombinedOutput()
		if err != nil {
			exitWithOutputAndError(repoAndImage, err)
		}
		env, err := app.envForServiceBindings()
		if err != nil {
			exitWithError(err)
		}
		newCmd := app.oc.Exec(app.createDeploymentArgs(string(repoAndImage), env)...)
		fmt.Printf("==> Creating deployment config with command: %s\n", newCmd.ArgsString())
		output, err := newCmd.CombinedOutput()
		fmt.Println(string(output))
		if err != nil {
			exitWithError(err)
		}
	} else {
		fmt.Printf("==> Deployment config already exists for %s, redeploying\n", app.Name)
		output, err := app.oc.Exec("deploy", app.Name, "--latest").CombinedOutput()
		if err != nil {
			exitWithOutputAndError(output, err)
		}
	}
}

func (app *Application) envForServiceBindings() ([]string, error) {
	var env []string
	var serviceNames []string
	if len(app.Services) > 0 {
		for _, service := range app.Services {
			envPrefix := envPrefixFromService(service)
			serviceNames = append(serviceNames, envPrefix)
			serviceEnv, err := app.envForServiceBinding(service, envPrefix)
			if err != nil {
				return nil, err
			}
			for key, value := range serviceEnv {
				env = append(env, fmt.Sprint(key, "=", value))
			}
		}
		env = append(env, fmt.Sprint(BoundServices, "=", strings.Join(serviceNames, " ")))
	}
	return env, nil
}

func (app *Application) envForServiceBinding(service string, envPrefix string) (map[string]string, error) {
	env := make(map[string]string)
	serviceEnv, err := app.oc.Env("dc", service)
	if err != nil {
		return nil, err
	}
	var label string
	for key, value := range serviceEnv {
		switch {
		case strings.HasPrefix(key, "POSTGRESQL"):
			label = "postgresql"
		case strings.HasPrefix(key, "MYSQL"):
			label = "mysql"
		case strings.HasPrefix(key, "MONGODB"):
			label = "mongodb"
		}
		switch {
		case strings.HasSuffix(key, "_USER"):
			env[fmt.Sprint(envPrefix, "_USER")] = value
		case strings.HasSuffix(key, "_PASSWORD"):
			env[fmt.Sprint(envPrefix, "_PASSWORD")] = value
		case strings.HasSuffix(key, "_DATABASE"):
			env[fmt.Sprint(envPrefix, "_DATABASE")] = value
		}
	}
	env[fmt.Sprint(envPrefix, "_LABEL")] = label
	return env, nil
}

func envPrefixFromService(service string) string {
	return strings.ToUpper(strings.Replace(service, "-", "_", -1))
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
	output, err := app.oc.Exec("get", "svc", app.Name).CombinedOutput()
	if strings.Contains(string(output), "not found") {
		newCmd := app.oc.Exec("expose", "dc", app.Name, "--port=8080")
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
	output, err := app.oc.Exec("get", "route", app.Name).CombinedOutput()
	if strings.Contains(string(output), "not found") {
		newCmd := app.oc.Exec("expose", "svc", app.Name)
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
	output, err := app.oc.Exec("get", "route", app.Name, "-o", "template",
		"--template={{.spec.host}}").CombinedOutput()
	if err != nil {
		exitWithOutputAndError(output, err)
	} else {
		fmt.Printf("==> Your application is available at %s\n", output)
	}
}

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func exitWithOutputAndError(output []byte, err error) {
	fmt.Println(string(output))
	exitWithError(err)
}
