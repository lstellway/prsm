package subcommand

import (
	"fmt"

	"github.com/spf13/cobra"
)

func ServeCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Start the HTTP/gRPC/MCP server",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("not yet implemented")
			return nil
		},
	}
}
