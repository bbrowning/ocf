package mocks

import (
	"github.com/stretchr/testify/mock"

	"github.com/bbrowning/ocf/pkg/exec"
)

type Execer struct {
	mock.Mock
}

func (execer *Execer) Oc(args ...string) exec.ExecCmd {
	mockArgs := execer.Called(args)
	return mockArgs.Get(0).(exec.ExecCmd)
}
