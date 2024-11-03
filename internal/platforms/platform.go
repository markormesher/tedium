package platforms

import (
	"fmt"

	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
)

var l = logging.Logger

type Platform interface {
	Init(conf *schema.TediumConfig) error
	Deinit() error

	Profile() *schema.PlatformProfile

	DiscoverRepos() ([]schema.Repo, error)
	RepoHasTediumConfig(repo *schema.Repo) (bool, error)
	ReadRepoFile(repo *schema.Repo, pathCandidates []string) ([]byte, error)
	OpenOrUpdatePullRequest(job *schema.Job) error
}

func FromConfig(conf *schema.TediumConfig, platformConfig *schema.PlatformConfig) (Platform, error) {
	switch platformConfig.Type {
	case "gitea":
		p, err := giteaPlatformFromConfig(conf, platformConfig)
		if err != nil {
			return nil, fmt.Errorf("Error building Gitea platform: %w", err)
		}
		return p, nil

	case "github":
		p, err := githubPlatformFromConfig(conf, platformConfig)
		if err != nil {
			return nil, fmt.Errorf("Error building GitHub platform: %w", err)
		}
		return p, nil
	}

	return nil, fmt.Errorf("Unrecognised platform type: %s", platformConfig.Type)
}
