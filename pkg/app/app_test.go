package app

import (
	"strings"
	"testing"

	"github.com/bbrowning/ocf/pkg/exec"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateBuildArgs(t *testing.T) {
	app := Application{Buildpack: "buildpack"}
	args := app.createBuildArgs("image")
	assert.Equal(t, "BUILDPACK_URL=buildpack", args[len(args)-1])
}

func TestCreateDeploymentArgs(t *testing.T) {
	cmd := "foobar baz"
	image := "foo"
	env := []string{}
	app := Application{Command: cmd}
	args := app.createDeploymentArgs(image, env)
	assertArgsContains(t, args, "CF_COMMAND=foobar baz")

	app.Memory = "2G"
	args = app.createDeploymentArgs(image, env)
	assertArgsContains(t, args, "MEMORY_LIMIT=2G,CF_COMMAND=foobar baz")
}

func TestEnvForServicesWithPostgres(t *testing.T) {
	execer := new(MockExecer)
	cmd := new(MockCmd)
	app := Application{execer: execer}
	app.Services = []string{"rails-postgres"}
	execer.On("Oc", []string{"env", "dc", "rails-postgres", "--list"}).Return(cmd)
	cmd.On("CombinedOutput").Return([]byte("POSTGRESQL_USER=foo\nPOSTGRESQL_PASSWORD=bar\nPOSTGRESQL_DATABASE=baz"), nil)
	env, err := app.envForServices()
	assert.Nil(t, err)
	assertArgsContains(t, env, "RAILS_POSTGRES_LABEL=postgresql")
	assertArgsContains(t, env, "RAILS_POSTGRES_USER=foo")
	assertArgsContains(t, env, "RAILS_POSTGRES_PASSWORD=bar")
	assertArgsContains(t, env, "RAILS_POSTGRES_DATABASE=baz")
	assertArgsContains(t, env, "CF_BOUND_SERVICES=RAILS_POSTGRES")
	execer.AssertExpectations(t)
	cmd.AssertExpectations(t)
}

func assertArgsContains(t *testing.T, args []string, expected string) {
	assert.Contains(t, strings.Join(args, " "), expected)
}

type MockExecer struct {
	mock.Mock
}

func (execer *MockExecer) Oc(args ...string) exec.ExecCmd {
	mockArgs := execer.Called(args)
	return mockArgs.Get(0).(exec.ExecCmd)
}

type MockCmd struct {
	mock.Mock
}

func (cmd *MockCmd) Run() error {
	args := cmd.Called()
	return args.Error(1)
}

func (cmd *MockCmd) CombinedOutput() ([]byte, error) {
	args := cmd.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (cmd *MockCmd) AttachStdIO() {
	cmd.Called()
}

func (cmd *MockCmd) ArgsString() string {
	args := cmd.Called()
	return args.String(0)
}
