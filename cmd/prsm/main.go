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
