package cmd

import (
	"errors"
	"fmt"

	"github.com/bbrowning/ocf/pkg/app"

	"github.com/spf13/cobra"
)

const (
	unbindCmdLong = `
Unbind a service from an application.

This command emulates Cloud Foundry's 'cf unbind-service' command but
targeting OpenShift instead. Not all the Cloud Foundry options are
supported; those that are supported are documented in the usage
information below.`

	unbindCmdExample = `
  # Unbind the 'rails-postgres' service from the application 'my-app'
  %[1]s unbind-service my-app rails-postgres`
)

type UnbindConfig struct {
	Application string
	Service     string
}

func init() {
	RootCmd.AddCommand(newUnbindCmd("ocf"))
}

func newUnbindCmd(commandName string) *cobra.Command {
	config := &UnbindConfig{}
	cmd := &cobra.Command{
		Use:     "unbind-service",
		Short:   "Unbind a service from an application.",
		Long:    unbindCmdLong,
		Example: fmt.Sprintf(unbindCmdExample, commandName),
		Run: func(cmd *cobra.Command, args []string) {
			err := config.Run(args)
			if err != nil {
				fmt.Printf("err: %v\n", err)
			}
		},
	}

	return cmd
}

func (config *UnbindConfig) Run(args []string) error {
	debugf("Config: %+v\n", config)

	if len(args) != 2 {
		return errors.New("Error: Application name and service name are required")
	}

	app := &app.Application{Name: args[0]}
	err := app.UnbindService(args[1])
	if err != nil {
		return err
	}

	return nil
}
