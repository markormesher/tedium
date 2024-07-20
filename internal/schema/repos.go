package schema

import "fmt"

// Repo represents a real Git repo, which may or may not be present on disk.
type Repo struct {
	// present for all repos
	CloneUrl string

	// present for target repos only
	OwnerName     string
	Name          string
	DefaultBranch string
	Archived      bool

	// present only if instantiated
	PathOnDisk string

	AuthConfig *AuthConfig
}

func (r *Repo) FullName() string {
	return fmt.Sprintf("%s/%s", r.OwnerName, r.Name)
}

// RepoConfig is read from a target repo. The main purpose is to define which chores are to be applied.
type RepoConfig struct {
	Extends []string          `json:"extends,omitempty"`
	Chores  []RepoChoreConfig `json:"chores,omitempty"`
}

// RepoChoreConfig defines one chore to apply to a repo.
type RepoChoreConfig struct {
	CloneUrl  string `json:"cloneUrl"`
	Directory string `json:"directory"`
	Config    any    `json:"config"`
}

// ResolvedRepoConfig is the result of taking a target repo, following all "extends" links, and resolving all chore references into their actual spec.s
type ResolvedRepoConfig struct {
	Chores []*ChoreSpec
}
