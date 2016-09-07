package mocks

import (
	"github.com/stretchr/testify/mock"

	"github.com/bbrowning/ocf/pkg/exec"
)

type Oc struct {
	mock.Mock
	Execer   Execer
	loggedIn bool
}

func NewMockOc() *Oc {
	return &Oc{
		loggedIn: true,
	}
}

func (oc *Oc) LoggedIn() bool {
	return oc.loggedIn
}

func (oc *Oc) Project() (string, error) {
	return "test-project", nil
}

func (oc *Oc) Exists(objType string, name string) (bool, error) {
	args := oc.Called(objType, name)
	return args.Bool(0), args.Error(1)
}

func (oc *Oc) NewBuild(image string, name string, env map[string]string) error {
	args := oc.Called(image, name, env)
	return args.Error(0)
}

func (oc *Oc) Env(objType string, name string) (map[string]string, error) {
	args := oc.Called(objType, name)
	return args.Get(0).(map[string]string), args.Error(1)
}

func (oc *Oc) SetEnv(objType string, name string, env map[string]string) error {
	args := oc.Called(objType, name, env)
	return args.Error(0)
}

func (oc *Oc) Exec(args ...string) exec.ExecCmd {
	return oc.Execer.Oc(args...)
}
