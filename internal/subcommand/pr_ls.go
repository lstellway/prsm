package subcommand

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/lstellway/prsm"
	"github.com/lstellway/prsm/config"
	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
	"github.com/spf13/cobra"
)

// PRListCommand fetches every configured pull-request source once and prints
// the resulting snapshot to stdout — the smallest end-to-end path from a real
// config file and a real token to real output, with no terminal UI involved.
// It exits 0 even when a connection is degraded or unreachable: a vendor
// outage is reported in the connections block, not treated as a command
// failure. Only a problem that leaves prsm with nothing to report at all — an
// unreadable config file, a nil client — is a fatal (non-zero) error.
func PRListCommand() *cobra.Command {
	var sortBy string
	var descending bool
	var format string

	listCommand := &cobra.Command{
		Use:     "ls",
		Aliases: []string{"list"},
		Short:   "Fetch and print one snapshot of pull requests",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return err
			}

			sortSpec, err := parseSortSpec(sortBy, descending)
			if err != nil {
				return err
			}

			parsedFormat, err := parseOutputFormat(format)
			if err != nil {
				return err
			}

			return runPRList(cmd, configPath, sortSpec, parsedFormat)
		},
	}

	listCommand.Flags().StringVar(&sortBy, "sort", "", "sort by updated|created|staleness|title (default: fetch order)")
	listCommand.Flags().BoolVar(&descending, "desc", false, "reverse sort direction (requires --sort)")
	listCommand.Flags().StringVar(&format, "format", "plain", "output format: plain|json")

	return listCommand
}

func runPRList(cmd *cobra.Command, configPath string, sortSpec query.SortSpec, format outputFormat) error {
	prsmConfig, err := config.LoadFile(configPath)
	if err != nil {
		return err
	}

	client, err := prsm.New(prsmConfig)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	return writePRListSnapshot(ctx, cmd.OutOrStdout(), client, sortSpec, format)
}

// writePRListSnapshot fetches once and writes the result, separated from
// runPRList so it can be exercised directly against a mock client — no
// config file, no cobra command, no real network call required.
func writePRListSnapshot(ctx context.Context, writer io.Writer, client *prsm.Client, sortSpec query.SortSpec, format outputFormat) error {
	snapshot := client.Fetch(ctx)
	pullRequests := query.Sort(snapshot.PullRequests, sortSpec)
	connections := connectionSummaries(client.ProviderStatuses(snapshot))

	if format == outputFormatJSON {
		return printSnapshotJSON(writer, snapshot.FetchedAt, pullRequests, connections)
	}
	printSnapshotPlain(writer, pullRequests, connections)
	return nil
}

// outputFormat is the closed set of values --format accepts.
type outputFormat string

const (
	outputFormatPlain outputFormat = "plain"
	outputFormatJSON  outputFormat = "json"
)

// parseOutputFormat validates the --format flag up front — before any config
// load or network call — the same way parseSortSpec validates --sort.
func parseOutputFormat(format string) (outputFormat, error) {
	switch outputFormat(format) {
	case outputFormatPlain, outputFormatJSON:
		return outputFormat(format), nil
	default:
		return "", fmt.Errorf("--format must be one of: plain, json")
	}
}

// parseSortSpec turns the --sort/--desc flag pair into a query.SortSpec.
// An empty sortBy is valid and means "leave the fetch order alone" — ls does
// not default to any particular ordering.
func parseSortSpec(sortBy string, descending bool) (query.SortSpec, error) {
	if sortBy == "" {
		if descending {
			return query.SortSpec{}, fmt.Errorf("--desc requires --sort")
		}
		return query.SortSpec{}, nil
	}

	switch query.SortKey(sortBy) {
	case query.SortUpdated, query.SortCreated, query.SortStaleness, query.SortTitle:
	default:
		return query.SortSpec{}, fmt.Errorf("--sort must be one of: updated, created, staleness, title")
	}

	direction := query.SortAsc
	if descending {
		direction = query.SortDesc
	}
	return query.SortSpec{By: query.SortKey(sortBy), Direction: direction}, nil
}

// connectionSummary is ls's text/JSON shape for one prsm.ProviderStatus —
// State flattened to its Label() and Err flattened to its Error() string,
// since both --format plain and --format json render text, not typed Go
// values. The merge that produces a ProviderStatus per configured provider
// (connection outcome plus never-constructed providers, in one view) lives on
// prsm.Client.ProviderStatuses so every consumer shares it; this type only
// shapes that result for printing here.
type connectionSummary struct {
	Provider string `json:"provider"`
	Kind     string `json:"kind"`
	State    string `json:"state"`
	Error    string `json:"error,omitempty"`
}

func connectionSummaries(providerStatuses []prsm.ProviderStatus) []connectionSummary {
	summaries := make([]connectionSummary, 0, len(providerStatuses))

	for _, providerStatus := range providerStatuses {
		summary := connectionSummary{
			Provider: providerStatus.Provider,
			Kind:     string(providerStatus.Kind),
			State:    providerStatus.Label(),
		}
		if providerStatus.Err != nil {
			summary.Error = providerStatus.Err.Error()
		}
		summaries = append(summaries, summary)
	}

	return summaries
}

// printSnapshotPlain writes one tab-separated line per pull request, followed
// by a connections block reporting every provider's outcome — ok, degraded,
// or never constructed. No column alignment: this output is meant to be
// grepped and diffed, not admired.
func printSnapshotPlain(writer io.Writer, pullRequests []model.PullRequest, connections []connectionSummary) {
	if len(pullRequests) == 0 {
		fmt.Fprintln(writer, "no pull requests found")
	}
	for _, pullRequest := range pullRequests {
		fmt.Fprintf(writer, "%s\t%s/%s\t#%d\t%s\t%s\t%s\t%s\n",
			pullRequest.Provider.Name,
			pullRequest.Repo.Owner, pullRequest.Repo.Name,
			pullRequest.Number,
			pullRequest.State,
			pullRequest.Author.Username,
			pullRequest.Title,
			pullRequest.URL,
		)
	}

	fmt.Fprintln(writer)
	fmt.Fprintln(writer, "CONNECTIONS")
	for _, connection := range connections {
		if connection.Error == "" {
			fmt.Fprintf(writer, "%s\t%s\t%s\n", connection.Provider, connection.Kind, connection.State)
			continue
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", connection.Provider, connection.Kind, connection.State, connection.Error)
	}
}

// snapshotDocument is the --format json wire shape. PullRequests and
// Connections always encode as [] rather than null, even when empty, so a
// script reading this output can always index into them without a null
// check.
type snapshotDocument struct {
	FetchedAt    time.Time           `json:"fetched_at"`
	PullRequests []model.PullRequest `json:"pull_requests"`
	Connections  []connectionSummary `json:"connections"`
}

func printSnapshotJSON(writer io.Writer, fetchedAt time.Time, pullRequests []model.PullRequest, connections []connectionSummary) error {
	if pullRequests == nil {
		pullRequests = []model.PullRequest{}
	}
	if connections == nil {
		connections = []connectionSummary{}
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(snapshotDocument{
		FetchedAt:    fetchedAt,
		PullRequests: pullRequests,
		Connections:  connections,
	})
}
