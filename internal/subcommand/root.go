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
