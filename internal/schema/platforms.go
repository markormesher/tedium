package schema

import (
	"regexp"
)

// PlatformConfig defines a Git platform from which repos can be discovered, such as Gitea or GitHub.
type PlatformConfig struct {
	Type     string      `json:"type"`
	Endpoint string      `json:"endpoint"`
	Auth     *AuthConfig `json:"auth"`

	// RepoFiltersRaw specifies a list of Go regexes; if specified, only repos that match at least one filter will be processed.
	RepoFiltersRaw []string `json:"repoFilters"`
	RepoFilters    []*regexp.Regexp

	// populated during execution, not via config
	BotProfile struct {
		Username string
		Email    string
	}
}

// AuthConfig defines how to authenticate with a platform.
type AuthConfig struct {
	Token string `json:"token"`
}

func (pc PlatformConfig) AcceptsRepo(fullName string) bool {
	if pc.RepoFilters == nil {
		return true
	}

	for i := range pc.RepoFilters {
		// loop by index to avoid copying the regexp object
		if pc.RepoFilters[i].MatchString(fullName) {
			return true
		}
	}

	return false
}

type PlatformBotProfile struct {
	Username string
	Email    string
}
