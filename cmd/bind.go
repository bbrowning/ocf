package cmd

import (
	"errors"
	"fmt"

	"github.com/bbrowning/ocf/pkg/app"

	"github.com/spf13/cobra"
)

const (
	bindCmdLong = `
Bind a service to an application.

This command emulates Cloud Foundry's 'cf bind-service' command but
targeting OpenShift instead. Not all the Cloud Foundry options are
supported; those that are supported are documented in the usage
information below.`

	bindCmdExample = `
  # Bind a 'rails-postgres' service to the application 'my-app'
  %[1]s bind-service my-app rails-postgres`
)

type BindConfig struct {
	Application string
	Service     string
}

func init() {
	RootCmd.AddCommand(newBindCmd("ocf"))
}

func newBindCmd(commandName string) *cobra.Command {
	config := &BindConfig{}
	cmd := &cobra.Command{
		Use:     "bind-service",
		Short:   "Bind a service to an application.",
		Long:    bindCmdLong,
		Example: fmt.Sprintf(bindCmdExample, commandName),
		Run: func(cmd *cobra.Command, args []string) {
			err := config.Run(args)
			if err != nil {
				fmt.Printf("err: %v\n", err)
			}
		},
	}

	return cmd
}

func (config *BindConfig) Run(args []string) error {
	debugf("Config: %+v\n", config)

	if len(args) != 2 {
		return errors.New("Error: Application name and service name are required")
	}

	app := &app.Application{Name: args[0]}
	err := app.BindService(args[1])
	if err != nil {
		return err
	}

	return nil
}
