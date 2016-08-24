package exec

import (
	"os"
	"os/exec"
	"strings"
)

type ExecCmd interface {
	Run() error
	CombinedOutput() ([]byte, error)
	AttachStdIO()
	ArgsString() string
}

type DefaultCmd struct {
	*exec.Cmd
}

func (cmd *DefaultCmd) AttachStdIO() {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func (cmd *DefaultCmd) ArgsString() string {
	return strings.Join(cmd.Args, " ")
}

type Execer interface {
	Oc(args ...string) ExecCmd
}

type DefaultExecer struct {
}

func (execer *DefaultExecer) Oc(args ...string) ExecCmd {
	return &DefaultCmd{exec.Command("oc", args...)}
}
