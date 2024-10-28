// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License.

package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/azure/azure-dev/cli/azd/cmd/actions"
	"github.com/azure/azure-dev/cli/azd/internal"
	"github.com/azure/azure-dev/cli/azd/pkg/contracts"
	"github.com/azure/azure-dev/cli/azd/pkg/input"
	"github.com/azure/azure-dev/cli/azd/pkg/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type versionFlags struct {
	global *internal.GlobalCommandOptions
}

func (v *versionFlags) Bind(local *pflag.FlagSet, global *internal.GlobalCommandOptions) {
	v.global = global
}

// Haozhan: Create an instance of VersionFlags and bind global options
func newVersionFlags(cmd *cobra.Command, global *internal.GlobalCommandOptions) *versionFlags {
	flags := &versionFlags{}
	flags.Bind(cmd.Flags(), global)

	return flags
}

type versionAction struct {
	flags     *versionFlags
	formatter output.Formatter
	writer    io.Writer
	console   input.Console
}

// Haozhan: Create a new instance of VersionAction
func newVersionAction(
	flags *versionFlags,
	formatter output.Formatter,
	writer io.Writer,
	console input.Console,
) actions.Action {
	return &versionAction{
		flags:     flags,
		formatter: formatter,
		writer:    writer,
		console:   console,
	}
}

// Haozhan: This method is automatically invoked by Cobra when executing the `azd version` command,
// 1. Resolve the versionAction using the newVersionAction function
// 2. Execute the Run method of the versionAction
func (v *versionAction) Run(ctx context.Context) (*actions.ActionResult, error) {
	switch v.formatter.Kind() {
	case output.NoneFormat:
		fmt.Fprintf(v.console.Handles().Stdout, "azd version %s\n", internal.Version)
	case output.JsonFormat:
		var result contracts.VersionResult
		versionSpec := internal.VersionInfo()

		result.Azd.Commit = versionSpec.Commit
		result.Azd.Version = versionSpec.Version.String()

		err := v.formatter.Format(result, v.writer, nil)
		if err != nil {
			return nil, err
		}
	}

	return nil, nil
}
