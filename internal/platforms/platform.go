package platforms

import (
	"fmt"

	"github.com/markormesher/tedium/internal/schema"
)

type Platform interface {
	Init(conf *schema.TediumConfig) error
	Deinit() error

	BotProfile() *schema.PlatformBotProfile

	DiscoverRepos() ([]schema.Repo, error)
	RepoHasTediumConfig(repo *schema.Repo) (bool, error)
	ReadRepoFile(repo *schema.Repo, path string) ([]byte, error)
	OpenOrUpdatePullRequest(job *schema.Job) error
}

var platformCache = make(map[string]Platform)

func FromConfig(config *schema.TediumConfig, endpoint string) (Platform, error) {
	cachedPlatform, ok := platformCache[endpoint]
	if ok {
		return cachedPlatform, nil
	}

	var platformConfig *schema.PlatformConfig
	for i := range config.Platforms {
		if config.Platforms[i].Endpoint == endpoint {
			platformConfig = config.Platforms[i]
			break
		}
	}

	if platformConfig == nil {
		return nil, fmt.Errorf("Unrecognised platform endpoint: %s", endpoint)
	}

	switch platformConfig.Type {
	case "gitea":
		p := &GiteaPlatform{
			Endpoint: platformConfig.Endpoint,
			Auth:     platformConfig.Auth,
		}
		platformCache[endpoint] = p
		return p, nil
	}

	return nil, fmt.Errorf("Unrecognised platform type: %s", platformConfig.Type)
}
