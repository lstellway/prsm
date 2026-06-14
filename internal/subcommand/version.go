package subcommand

import (
	"fmt"

	"github.com/spf13/cobra"
)

var Version = "dev"

func VersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("prsm %s\n", Version)
		},
	}
}
