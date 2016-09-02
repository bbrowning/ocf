package oc

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bbrowning/ocf/pkg/exec"
)

type Oc interface {
	LoggedIn() bool
	Env(string, string) (map[string]string, error)
	Project() (string, error)
	Exists(string, string) (bool, error)
	SetEnv(string, string, map[string]string) error
	Exec(args ...string) exec.ExecCmd
}

type DefaultOc struct {
	execer exec.Execer
}

func (oc *DefaultOc) LoggedIn() bool {
	err := oc.Exec("whoami").Run()
	if err != nil {
		return false
	}
	return true
}

func (oc *DefaultOc) Project() (string, error) {
	output, err := oc.Exec("project", "-q").CombinedOutput()
	return string(output), err
}

func (oc *DefaultOc) Exists(objType string, name string) (bool, error) {
	output, err := oc.Exec("get", objType, name).CombinedOutput()
	if strings.Contains(string(output), "not found") {
		return false, nil
	} else if err != nil {
		return false, errors.New(fmt.Sprintf("Error getting %s %s: %s\n", objType, name, output))
	} else {
		return true, nil
	}
}

func (oc *DefaultOc) Env(objType string, name string) (map[string]string, error) {
	var env = make(map[string]string)
	output, err := oc.Exec("env", objType, name, "--list").CombinedOutput()
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Error: %s %s not found\n", objType, name))
	}
	for _, line := range strings.Split(string(output), "\n") {
		split := strings.Split(line, "=")
		if len(split) == 2 {
			env[split[0]] = split[1]
		}
	}
	return env, nil
}

func (oc *DefaultOc) SetEnv(objType string, name string, env map[string]string) error {
	envList := []string{}
	for key, value := range env {
		envList = append(envList, fmt.Sprint(key, "=", value))
	}
	execArgs := []string{"env", objType, name}
	execArgs = append(execArgs, envList...)
	envCmd := oc.Exec(execArgs...)
	fmt.Printf("==> Updating environment variables with command: %s\n", envCmd.ArgsString())
	output, err := envCmd.CombinedOutput()
	if err != nil {
		return errors.New(fmt.Sprintf("Error updating environment: %s\n", output))
	}
	return nil
}

func (oc *DefaultOc) Exec(args ...string) exec.ExecCmd {
	if oc.execer == nil {
		oc.execer = new(exec.DefaultExecer)
	}
	return oc.execer.Oc(args...)
}
