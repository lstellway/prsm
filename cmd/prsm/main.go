// Command prsm is the CLI entry point; it delegates entirely to
// internal/subcommand.Root().
package main

import (
	"os"

	"github.com/lstellway/prsm/internal/subcommand"
)

func main() {
	if err := subcommand.Root().Execute(); err != nil {
		os.Exit(1)
	}
}
