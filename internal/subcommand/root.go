// Package subcommand defines prsm's cobra command tree — tui (default),
// serve, version, and the per-resource-kind pr command family. It is a thin
// CLI consumer: it loads config, constructs a prsm.Client, and renders
// query/model results as text or JSON; no filtering, sorting, or fetching
// logic lives here.
package subcommand

import "github.com/spf13/cobra"

func Root() *cobra.Command {
	tuiCommand := TUICommand()
	rootCommand := &cobra.Command{
		Use:   "prsm",
		Short: "PR inbox for engineers on fast-moving teams",
		RunE:  tuiCommand.RunE,
	}
	rootCommand.PersistentFlags().String("config", "", "path to the prsm config file (default: XDG config path)")
	rootCommand.AddCommand(tuiCommand, ServeCommand(), VersionCommand(), PRCommand())
	return rootCommand
}
