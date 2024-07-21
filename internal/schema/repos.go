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
