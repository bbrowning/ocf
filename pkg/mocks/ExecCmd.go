package mocks

import (
	"strings"

	"github.com/stretchr/testify/mock"
)

type ExecCmd struct {
	mock.Mock
	Args []string
}

func (cmd *ExecCmd) Run() error {
	args := cmd.Called()
	return args.Error(0)
}

func (cmd *ExecCmd) CombinedOutput() ([]byte, error) {
	args := cmd.Called()
	return args.Get(0).([]byte), args.Error(1)
}

func (cmd *ExecCmd) AttachStdIO() {
	cmd.Called()
}

func (cmd *ExecCmd) ArgsString() string {
	return strings.Join(cmd.Args, " ")
}
