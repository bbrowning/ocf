package oc

import (
	"errors"
	"strings"
	"testing"

	"github.com/bbrowning/ocf/pkg/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type execHandler func(*DefaultOc, *mocks.ExecCmd)

func TestLoggedInTrue(t *testing.T) {
	withSingleExec(t, []string{"whoami"}, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("Run").Return(nil)
		assert.True(t, oc.LoggedIn())
	})
}

func TestLoggedInFalse(t *testing.T) {
	withSingleExec(t, []string{"whoami"}, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("Run").Return(errors.New("error"))
		assert.False(t, oc.LoggedIn())
	})
}

func TestProject(t *testing.T) {
	withSingleExec(t, []string{"project", "-q"}, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("CombinedOutput").Return([]byte("test-project"), nil)
		project, err := oc.Project()
		assert.Nil(t, err)
		assert.Equal(t, "test-project", project)
	})
}

func TestExistsTrue(t *testing.T) {
	withSingleExec(t, []string{"get", "dc", "foo"}, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("CombinedOutput").Return([]byte(""), nil)
		exists, err := oc.Exists("dc", "foo")
		assert.Nil(t, err)
		assert.True(t, exists)
	})
}

func TestExistsFalse(t *testing.T) {
	withSingleExec(t, []string{"get", "dc", "foo"}, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("CombinedOutput").Return([]byte("not found"), errors.New(""))
		exists, err := oc.Exists("dc", "foo")
		assert.Nil(t, err)
		assert.False(t, exists)
	})
}

func TestExistsError(t *testing.T) {
	withSingleExec(t, []string{"get", "dc", "foo"}, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("CombinedOutput").Return([]byte("error"), errors.New(""))
		_, err := oc.Exists("dc", "foo")
		assert.NotNil(t, err)
	})
}

func TestEnvHappyPath(t *testing.T) {
	execArgs := []string{"env", "dc", "foo", "--list"}
	withSingleExec(t, execArgs, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("CombinedOutput").Return([]byte("FOO=bar\nBAZ=blah"), nil)
		env, err := oc.Env("dc", "foo")
		assert.Nil(t, err)
		assert.Equal(t, "bar", env["FOO"])
		assert.Equal(t, "blah", env["BAZ"])
	})
}

func TestEnvNotFound(t *testing.T) {
	execArgs := []string{"env", "dc", "foo", "--list"}
	withSingleExec(t, execArgs, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("CombinedOutput").Return([]byte(""), errors.New(""))
		_, err := oc.Env("dc", "foo")
		assert.NotNil(t, err)
	})
}

func TestEnvNoneSet(t *testing.T) {
	execArgs := []string{"env", "dc", "foo", "--list"}
	withSingleExec(t, execArgs, func(oc *DefaultOc, cmd *mocks.ExecCmd) {
		cmd.On("CombinedOutput").Return([]byte("# deploymentconfigs foo, container foo"), nil)
		env, err := oc.Env("dc", "foo")
		assert.Nil(t, err)
		assert.Equal(t, 0, len(env))
	})
}

func TestSetEnvHappyPath(t *testing.T) {
	execer := &mocks.Execer{}
	cmd := &mocks.ExecCmd{}
	execer.On("Oc", mock.MatchedBy(func(args []string) bool {
		argsStr := strings.Join(args, " ")
		return strings.HasPrefix(argsStr, "env dc foo") &&
			strings.Contains(argsStr, "FOO=bar") &&
			strings.Contains(argsStr, "BAZ=blah") &&
			strings.Contains(argsStr, "DELETED-")
	})).Return(cmd)
	cmd.On("CombinedOutput").Return([]byte(""), nil)
	oc := &DefaultOc{
		execer: execer,
	}

	err := oc.SetEnv("dc", "foo", map[string]string{
		"FOO":     "bar",
		"BAZ":     "blah",
		"DELETED": "-",
	})
	assert.Nil(t, err)
	execer.AssertExpectations(t)
	cmd.AssertExpectations(t)
}

func withSingleExec(t *testing.T, args []string, handler execHandler) {
	execer := &mocks.Execer{}
	cmd := &mocks.ExecCmd{Args: args}
	execer.On("Oc", args).Return(cmd)
	oc := &DefaultOc{
		execer: execer,
	}
	handler(oc, cmd)
	execer.AssertExpectations(t)
	cmd.AssertExpectations(t)
}
