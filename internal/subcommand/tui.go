package subcommand

import (
	"fmt"

	"github.com/spf13/cobra"
)

func TUICommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the terminal UI (default)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("prsm: TUI not yet implemented")
			return nil
		},
	}
}
