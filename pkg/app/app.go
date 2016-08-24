package app

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/bbrowning/ocf/pkg/exec"
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
	execer    exec.Execer
}

func (app *Application) Push(image string) {
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

func (app *Application) SetupDefaults() {
	if app.execer == nil {
		app.execer = new(exec.DefaultExecer)
	}
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

func exitWithError(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func exitWithOutputAndError(output []byte, err error) {
	fmt.Println(string(output))
	exitWithError(err)
}
