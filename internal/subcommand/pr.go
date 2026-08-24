package subcommand

import "github.com/spf13/cobra"

// PRCommand groups every pull-request verb under one resource-typed parent —
// ls today, with merge/comment/label/etc. joining it later per CLAUDE.md's
// "manage, don't just watch" principle. Resource kinds are first-class equals
// in prsm's design, so each one (pr today; action, issue, branch, worktree
// later) gets its own top-level namespace instead of a flat command list that
// would need restructuring as they land.
func PRCommand() *cobra.Command {
	prCommand := &cobra.Command{
		Use:     "pr",
		Aliases: []string{"pullrequest", "pull-request"},
		Short:   "Work with pull requests",
	}
	prCommand.AddCommand(PRListCommand())
	return prCommand
}
