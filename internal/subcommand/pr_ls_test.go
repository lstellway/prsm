package subcommand

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/lstellway/prsm"
	"github.com/lstellway/prsm/adapter/mock"
	"github.com/lstellway/prsm/model"
	"github.com/lstellway/prsm/query"
)

func TestParseSortSpec(t *testing.T) {
	testCases := []struct {
		name       string
		sortBy     string
		descending bool
		want       query.SortSpec
		wantErr    bool
	}{
		{name: "empty means unsorted", sortBy: "", descending: false, want: query.SortSpec{}},
		{name: "desc without sort is an error", sortBy: "", descending: true, wantErr: true},
		{name: "valid key defaults to ascending", sortBy: "updated", descending: false,
			want: query.SortSpec{By: query.SortUpdated, Direction: query.SortAsc}},
		{name: "valid key with desc", sortBy: "staleness", descending: true,
			want: query.SortSpec{By: query.SortStaleness, Direction: query.SortDesc}},
		{name: "unknown key is an error", sortBy: "popularity", descending: false, wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseSortSpec(testCase.sortBy, testCase.descending)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got spec %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Errorf("got %+v, want %+v", got, testCase.want)
			}
		})
	}
}

func TestConnectionSummaries_MergesFetchedAndConstructFailed(t *testing.T) {
	statuses := []prsm.ConnectionStatus{
		{
			Provider: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub},
			State:    prsm.ConnectionStateOK,
		},
		{
			Provider: model.ProviderInstance{Name: "github-work", Kind: model.ProviderGitHub},
			State:    prsm.ConnectionStateOffline,
			Err:      errors.New("dial tcp: i/o timeout"),
		},
	}
	failedProviders := []*prsm.ConstructError{
		{Provider: "gitlab-work", Kind: model.ProviderGitLab, Reason: prsm.ConstructErrorReasonNotImplemented},
	}

	summaries := connectionSummaries(statuses, failedProviders)

	if len(summaries) != 3 {
		t.Fatalf("got %d summaries, want 3: %+v", len(summaries), summaries)
	}

	if got := summaries[0]; got.Provider != "github-personal" || got.State != "ok" || got.Error != "" {
		t.Errorf("summaries[0] = %+v", got)
	}
	if got := summaries[1]; got.Provider != "github-work" || got.State != "offline" || got.Error != "dial tcp: i/o timeout" {
		t.Errorf("summaries[1] = %+v", got)
	}
	if got := summaries[2]; got.Provider != "gitlab-work" || got.State != "not_implemented" || got.Error == "" {
		t.Errorf("summaries[2] = %+v", got)
	}
}

func TestPrintSnapshotPlain_NoPullRequests(t *testing.T) {
	var buffer bytes.Buffer
	printSnapshotPlain(&buffer, nil, []connectionSummary{
		{Provider: "github-personal", Kind: "github", State: "ok"},
	})

	output := buffer.String()
	if !strings.Contains(output, "no pull requests found") {
		t.Errorf("expected an explicit empty-result line, got:\n%s", output)
	}
	if !strings.Contains(output, "github-personal\tgithub\tok") {
		t.Errorf("expected connection line, got:\n%s", output)
	}
}

func TestPrintSnapshotPlain_PullRequestLineAndDegradedConnection(t *testing.T) {
	var buffer bytes.Buffer
	pullRequests := []model.PullRequest{
		{
			Provider: model.ProviderInstance{Name: "github-personal"},
			Repo:     model.Repository{Owner: "lstellway", Name: "prsm"},
			Number:   14,
			State:    model.PRStateOpen,
			Author:   model.Author{Username: "loganstellway"},
			Title:    "prsm ls: print one snapshot to stdout",
			URL:      "https://github.com/lstellway/prsm/pull/14",
		},
	}
	connections := []connectionSummary{
		{Provider: "gitlab-work", Kind: "gitlab", State: "unauthorized", Error: "authentication failed"},
	}

	printSnapshotPlain(&buffer, pullRequests, connections)

	output := buffer.String()
	wantPRLine := "github-personal\tlstellway/prsm\t#14\topen\tloganstellway\tprsm ls: print one snapshot to stdout\thttps://github.com/lstellway/prsm/pull/14"
	if !strings.Contains(output, wantPRLine) {
		t.Errorf("missing PR line in output:\n%s", output)
	}
	wantConnLine := "gitlab-work\tgitlab\tunauthorized\tauthentication failed"
	if !strings.Contains(output, wantConnLine) {
		t.Errorf("missing connection line in output:\n%s", output)
	}
}

func TestPrintSnapshotJSON_EncodesEmptyListsNotNull(t *testing.T) {
	var buffer bytes.Buffer
	if err := printSnapshotJSON(&buffer, time.Now(), nil, nil); err != nil {
		t.Fatalf("printSnapshotJSON: %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	for _, field := range []string{"pull_requests", "connections"} {
		raw, ok := decoded[field]
		if !ok {
			t.Fatalf("missing field %q in %s", field, buffer.String())
		}
		if strings.TrimSpace(string(raw)) != "[]" {
			t.Errorf("field %q = %s, want []", field, raw)
		}
	}
}

func TestPrintSnapshotJSON_RoundTrips(t *testing.T) {
	var buffer bytes.Buffer
	fetchedAt := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	pullRequests := []model.PullRequest{
		{Title: "example", CommentCount: model.Loaded(0)},
	}
	connections := []connectionSummary{
		{Provider: "github-personal", Kind: "github", State: "ok"},
	}

	if err := printSnapshotJSON(&buffer, fetchedAt, pullRequests, connections); err != nil {
		t.Fatalf("printSnapshotJSON: %v", err)
	}

	var decoded snapshotDocument
	if err := json.Unmarshal(buffer.Bytes(), &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.FetchedAt.Equal(fetchedAt) {
		t.Errorf("FetchedAt = %v, want %v", decoded.FetchedAt, fetchedAt)
	}
	if len(decoded.PullRequests) != 1 || decoded.PullRequests[0].Title != "example" {
		t.Errorf("PullRequests = %+v", decoded.PullRequests)
	}
	if len(decoded.Connections) != 1 || decoded.Connections[0].Provider != "github-personal" {
		t.Errorf("Connections = %+v", decoded.Connections)
	}
}

// TestWritePRListSnapshot_DegradedConnectionExitsCleanly is the behavior
// issue #14 hinges on: a connection that fails must still produce a
// zero-value error from the command (cobra maps a non-nil RunE error to a
// nonzero exit code), with the failure surfaced in the connections block
// instead.
func TestWritePRListSnapshot_DegradedConnectionExitsCleanly(t *testing.T) {
	client := prsm.NewWithConnections(
		&mock.PullRequestSource{
			Connection:      mock.Connection{InstanceVal: model.ProviderInstance{Name: "github-personal", Kind: model.ProviderGitHub}},
			PullRequestsErr: errors.New("network unreachable"),
		},
	)

	var buffer bytes.Buffer
	if err := writePRListSnapshot(context.Background(), &buffer, client, query.SortSpec{}, outputFormatPlain); err != nil {
		t.Fatalf("expected nil error for a degraded (not fatal) connection, got %v", err)
	}

	output := buffer.String()
	if !strings.Contains(output, "github-personal\tgithub\toffline\tnetwork unreachable") {
		t.Errorf("expected degraded connection reported in output, got:\n%s", output)
	}
}

func TestParseOutputFormat(t *testing.T) {
	testCases := []struct {
		name    string
		format  string
		want    outputFormat
		wantErr bool
	}{
		{name: "plain", format: "plain", want: outputFormatPlain},
		{name: "json", format: "json", want: outputFormatJSON},
		{name: "unrecognized value is an error", format: "yaml", wantErr: true},
		{name: "empty value is an error", format: "", wantErr: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, err := parseOutputFormat(testCase.format)
			if testCase.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != testCase.want {
				t.Errorf("got %q, want %q", got, testCase.want)
			}
		})
	}
}
