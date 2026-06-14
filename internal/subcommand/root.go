package subcommand

import "github.com/spf13/cobra"

func Root() *cobra.Command {
	tui := TUICommand()
	root := &cobra.Command{
		Use:   "prsm",
		Short: "PR inbox for engineers on fast-moving teams",
		RunE:  tui.RunE,
	}
	root.AddCommand(tui, ServeCommand(), VersionCommand())
	return root
}
