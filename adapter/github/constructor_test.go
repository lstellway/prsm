package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	gogithub "github.com/google/go-github/v88/github"
	"github.com/lstellway/prsm/adapter"
	"github.com/lstellway/prsm/model"
)

// ---------------------------------------------------------------------------
// TestNew
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	cases := []struct {
		name          string
		adapterConfig Config
		wantErr       bool
		wantHost      string
		wantKind      model.ProviderKind
	}{
		{
			name:          "empty_token",
			adapterConfig: Config{Name: "test", Token: ""},
			wantErr:       true,
		},
		{
			name:          "valid_github_com",
			adapterConfig: Config{Name: "test", Token: "ghp_test_token"},
			wantErr:       false,
			wantHost:      "github.com",
			wantKind:      model.ProviderGitHub,
		},
		{
			name:          "enterprise_url",
			adapterConfig: Config{Name: "test", Token: "ghp_test_token", BaseURL: "https://ghe.example.com/api/v3"},
			wantErr:       false,
			wantHost:      "ghe.example.com",
			wantKind:      model.ProviderGitHub,
		},
		{
			name:          "enterprise_url_with_port",
			adapterConfig: Config{Name: "test", Token: "ghp_test_token", BaseURL: "https://ghe.example.com:8443"},
			wantErr:       false,
			wantHost:      "ghe.example.com",
			wantKind:      model.ProviderGitHub,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			githubAdapter, err := New(testCase.adapterConfig)
			if testCase.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if githubAdapter == nil {
				t.Fatal("expected non-nil adapter")
			}
			instance := githubAdapter.Instance()
			if instance.Host != testCase.wantHost {
				t.Errorf("Instance().Host = %q, want %q", instance.Host, testCase.wantHost)
			}
			if instance.Kind != testCase.wantKind {
				t.Errorf("Instance().Kind = %q, want %q", instance.Kind, testCase.wantKind)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestExtractHost
// ---------------------------------------------------------------------------

func TestExtractHost(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "https_no_path",
			input: "https://github.com",
			want:  "github.com",
		},
		{
			name:  "https_with_path",
			input: "https://github.com/owner/repo",
			want:  "github.com",
		},
		{
			name:  "https_with_port",
			input: "https://ghe.example.com:8443/api/v3",
			want:  "ghe.example.com",
		},
		{
			name:  "http_scheme",
			input: "http://evil.com/path",
			want:  "evil.com",
		},
		{
			name:  "empty_string",
			input: "",
			want:  "",
		},
		{
			name:  "no_host_in_parsed_url",
			input: "not-a-url",
			want:  "not-a-url",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := extractHost(testCase.input)
			if got != testCase.want {
				t.Errorf("extractHost(%q) = %q, want %q", testCase.input, got, testCase.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestListRepoPRsPagination
// ---------------------------------------------------------------------------

// minimalGHPR is the minimal JSON structure needed so go-github can unmarshal
// a pull request from the list endpoint.
type minimalGHPR struct {
	NodeID string `json:"node_id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	Head   struct {
		SHA  string `json:"sha"`
		Ref  string `json:"ref"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Ref string `json:"ref"`
	} `json:"base"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

func makePR(id int) minimalGHPR {
	pullRequest := minimalGHPR{
		NodeID: fmt.Sprintf("pr%d", id),
		Number: id,
		Title:  fmt.Sprintf("PR %d", id),
		State:  "open",
	}
	pullRequest.Head.SHA = fmt.Sprintf("sha%d", id)
	pullRequest.Head.Ref = "feat"
	pullRequest.Head.Repo.FullName = "owner/repo"
	pullRequest.Base.Ref = "main"
	pullRequest.User.Login = "alice"
	return pullRequest
}

// newTestClient returns a *gogithub.Client pointed at the given test server URL.
// It uses the standard pattern from go-github's own test suite: create a client,
// then overwrite BaseURL and UploadURL with the test server's URL.
func newTestClient(serverURL string) (*gogithub.Client, error) {
	baseURL := serverURL + "/"
	return gogithub.NewClient(
		gogithub.WithAuthToken("fake-token"),
		gogithub.WithEnterpriseURLs(baseURL, baseURL),
	)
}

func TestListRepoPRsPagination(t *testing.T) {
	t.Run("multi_page_concatenation", func(t *testing.T) {
		// Page 1: PRs 1–2 with a Link header pointing to page 2.
		// Page 2: PRs 3–4 with no Link header (last page).
		page1PullRequests := []minimalGHPR{makePR(1), makePR(2)}
		page2PullRequests := []minimalGHPR{makePR(3), makePR(4)}

		server := httptest.NewServer(http.HandlerFunc(
			func(responseWriter http.ResponseWriter, request *http.Request) {
				page := request.URL.Query().Get("page")
				if page == "" || page == "1" {
					nextURL := fmt.Sprintf("<%s?page=2>; rel=\"next\"", request.URL.Path)
					responseWriter.Header().Set("Link", nextURL)
					responseWriter.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(responseWriter).Encode(page1PullRequests); err != nil {
						t.Errorf("encode page1: %v", err)
					}
				} else {
					responseWriter.Header().Set("Content-Type", "application/json")
					if err := json.NewEncoder(responseWriter).Encode(page2PullRequests); err != nil {
						t.Errorf("encode page2: %v", err)
					}
				}
			}))
		defer server.Close()

		restClient, err := newTestClient(server.URL)
		if err != nil {
			t.Fatalf("newTestClient: %v", err)
		}

		githubAdapter := &GitHubAdapter{
			providerName: "test",
			instance: model.ProviderInstance{
				Name: "test",
				Kind: model.ProviderGitHub,
				Host: "localhost",
			},
			repos: []adapter.RepoRef{{Owner: "owner", Repo: "repo"}},
			rest:  restClient,
		}

		pullRequests, err := githubAdapter.listRepoPullRequests(context.Background(), "owner", "repo")
		if err != nil {
			t.Fatalf("listRepoPullRequests: %v", err)
		}
		if len(pullRequests) != 4 {
			t.Errorf("got %d PRs, want 4", len(pullRequests))
		}
	})

	t.Run("page_cap_returns_error", func(t *testing.T) {
		// Always respond with a non-empty page and a next-page link so the
		// adapter never naturally terminates — it must hit the maxPages cap.
		pullRequest := makePR(1)
		body, _ := json.Marshal([]minimalGHPR{pullRequest})

		server := httptest.NewServer(http.HandlerFunc(
			func(responseWriter http.ResponseWriter, request *http.Request) {
				page := request.URL.Query().Get("page")
				if page == "" {
					page = "1"
				}
				// Always advertise the next page to prevent natural termination.
				nextURL := fmt.Sprintf("<%s?page=%s>; rel=\"next\"", request.URL.Path, page)
				responseWriter.Header().Set("Link", nextURL)
				responseWriter.Header().Set("Content-Type", "application/json")
				responseWriter.Write(body) //nolint:errcheck
			}))
		defer server.Close()

		restClient, err := newTestClient(server.URL)
		if err != nil {
			t.Fatalf("newTestClient: %v", err)
		}

		githubAdapter := &GitHubAdapter{
			providerName: "test",
			instance: model.ProviderInstance{
				Name: "test",
				Kind: model.ProviderGitHub,
				Host: "localhost",
			},
			repos: []adapter.RepoRef{{Owner: "owner", Repo: "repo"}},
			rest:  restClient,
		}

		_, err = githubAdapter.listRepoPullRequests(context.Background(), "owner", "repo")
		if err == nil {
			t.Fatal("expected error from page cap, got nil")
		}
	})
}
