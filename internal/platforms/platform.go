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

	BotProfile() *schema.PlatformBotProfile

	DiscoverRepos() ([]schema.Repo, error)
	RepoHasTediumConfig(repo *schema.Repo) (bool, error)
	ReadRepoFile(repo *schema.Repo, pathCandidates []string) ([]byte, error)
	OpenOrUpdatePullRequest(job *schema.Job) error
}

func FromConfig(platformConfig *schema.PlatformConfig) (Platform, error) {
	switch platformConfig.Type {
	case "gitea":
		p := &GiteaPlatform{
			// TODO: make this neater - can we have something like GiteaPlatform.from(...) ?
			originalPlatformConfig: platformConfig,
			Endpoint:               platformConfig.Endpoint,
			Auth:                   platformConfig.Auth,
		}
		return p, nil

	case "github":
		p := &GitHubPlatform{
			originalPlatformConfig: platformConfig,
			Endpoint:               platformConfig.Endpoint,
			Auth:                   platformConfig.Auth,
		}
		return p, nil
	}

	return nil, fmt.Errorf("Unrecognised platform type: %s", platformConfig.Type)
}
