package model

// DiffStats holds size metrics that require the PR detail endpoint.
type DiffStats struct {
	Commits      int
	ChangedFiles int
	Additions    int
	Deletions    int
}
