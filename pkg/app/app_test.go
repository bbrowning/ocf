package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/bbrowning/ocf/pkg/mocks"
)

func TestEnsureBuildExistsWhenDoesnt(t *testing.T) {
	oc := new(mocks.Oc)
	oc.On("Exists", "bc", "foo").Return(false, nil)
	oc.On("NewBuild", "my-image", "foo", mock.AnythingOfType("map[string]string")).Return(nil)
	app := Application{oc: oc, Name: "foo"}
	app.ensureBuildExists("my-image")
	oc.AssertExpectations(t)
}

func TestEnsureBuildExistsWhenDoesntWithBuildpack(t *testing.T) {
	oc := new(mocks.Oc)
	oc.On("Exists", "bc", "foo").Return(false, nil)
	oc.On("NewBuild", "my-image", "foo", map[string]string{BuildpackUrl: "bp"}).Return(nil)
	app := Application{oc: oc, Name: "foo", Buildpack: "bp"}
	app.ensureBuildExists("my-image")
	oc.AssertExpectations(t)
}

func TestEnsureBuildExistsDoesntSetEnvIfNotChanged(t *testing.T) {
	oc := new(mocks.Oc)
	oc.On("Exists", "bc", "foo").Return(true, nil)
	currentEnv := map[string]string{
		BuildpackUrl: "bp",
	}
	oc.On("Env", "bc", "foo").Return(currentEnv, nil)
	app := Application{oc: oc, Name: "foo", Buildpack: "bp"}
	app.ensureBuildExists("my-image")
	oc.AssertExpectations(t)
}

func TestEnsureBuildExistsCanUpdateBuildpack(t *testing.T) {
	oc := new(mocks.Oc)
	oc.On("Exists", "bc", "foo").Return(true, nil)
	currentEnv := map[string]string{
		BuildpackUrl: "bp1",
	}
	oc.On("Env", "bc", "foo").Return(currentEnv, nil)
	expectedEnv := map[string]string{
		BuildpackUrl: "bp2",
	}
	oc.On("SetEnv", "bc", "foo", expectedEnv).Return(nil)

	app := Application{oc: oc, Name: "foo", Buildpack: "bp2"}
	app.ensureBuildExists("my-image")
	oc.AssertExpectations(t)
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
	oc := new(mocks.Oc)
	app := Application{oc: oc}
	app.Services = []string{"rails-postgres"}
	mockEnv := map[string]string{
		"POSTGRESQL_USER":     "foo",
		"POSTGRESQL_PASSWORD": "bar",
		"POSTGRESQL_DATABASE": "baz",
	}
	oc.On("Env", "dc", "rails-postgres").Return(mockEnv, nil)
	env, err := app.envForServiceBindings()
	assert.Nil(t, err)
	assertArgsContains(t, env, "RAILS_POSTGRES_LABEL=postgresql")
	assertArgsContains(t, env, "RAILS_POSTGRES_USER=foo")
	assertArgsContains(t, env, "RAILS_POSTGRES_PASSWORD=bar")
	assertArgsContains(t, env, "RAILS_POSTGRES_DATABASE=baz")
	assertArgsContains(t, env, fmt.Sprint(BoundServices, "=RAILS_POSTGRES"))
	oc.AssertExpectations(t)
}

func TestEnvForServicesWithMysql(t *testing.T) {
	oc := new(mocks.Oc)
	app := Application{oc: oc}
	app.Services = []string{"rails-mysql"}
	mockEnv := map[string]string{
		"MYSQL_USER":     "foo",
		"MYSQL_PASSWORD": "bar",
		"MYSQL_DATABASE": "baz",
	}
	oc.On("Env", "dc", "rails-mysql").Return(mockEnv, nil)
	env, err := app.envForServiceBindings()
	assert.Nil(t, err)
	assertArgsContains(t, env, "RAILS_MYSQL_LABEL=mysql")
	assertArgsContains(t, env, "RAILS_MYSQL_USER=foo")
	assertArgsContains(t, env, "RAILS_MYSQL_PASSWORD=bar")
	assertArgsContains(t, env, "RAILS_MYSQL_DATABASE=baz")
	assertArgsContains(t, env, fmt.Sprint(BoundServices, "=RAILS_MYSQL"))
	oc.AssertExpectations(t)
}

func TestBindServiceSimpleHappyPath(t *testing.T) {
	oc := mocks.NewMockOc()
	app := Application{oc: oc, Name: "foo"}

	serviceEnv := map[string]string{
		"MYSQL_USER": "bar",
	}

	existingEnv := map[string]string{
		BoundServices: "SOME_SERVICE",
	}

	oc.On("Exists", "dc", "foo").Return(true, nil)
	oc.On("Env", "dc", "test-service").Return(serviceEnv, nil)
	oc.On("Env", "dc", "foo").Return(existingEnv, nil)

	expectedEnv := map[string]string{
		"TEST_SERVICE_USER":  "bar",
		"TEST_SERVICE_LABEL": "mysql",
		BoundServices:        "SOME_SERVICE TEST_SERVICE",
	}
	oc.On("SetEnv", "dc", "foo", expectedEnv).Return(nil)

	err := app.BindService("test-service")
	assert.Nil(t, err)
	oc.Execer.AssertExpectations(t)
}

func TestUnbindServiceHappyPath(t *testing.T) {
	oc := mocks.NewMockOc()
	app := Application{oc: oc, Name: "foo"}

	existingEnv := map[string]string{
		"FOO":                   "bar",
		BoundServices:           "TEST_SERVICE SOME_SERVICE",
		"TEST_SERVICE_LABEL":    "test-service",
		"TEST_SERVICE_DATABASE": "test-database",
	}

	oc.On("Exists", "dc", "foo").Return(true, nil)
	oc.On("Env", "dc", "foo").Return(existingEnv, nil)

	expectedEnv := map[string]string{
		BoundServices:           "SOME_SERVICE",
		"TEST_SERVICE_LABEL":    "-",
		"TEST_SERVICE_DATABASE": "-",
	}
	oc.On("SetEnv", "dc", "foo", expectedEnv).Return(nil)

	err := app.UnbindService("test-service")
	assert.Nil(t, err)
}

func assertArgsContains(t *testing.T, args []string, expected string) {
	assert.Contains(t, strings.Join(args, " "), expected)
}
