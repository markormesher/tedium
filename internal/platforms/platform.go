package platforms

import (
	"fmt"

	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
)

var l = logging.Logger

var platformCache []Platform

type Platform interface {
	Init(conf *schema.TediumConfig) error
	Deinit() error

	AcceptsDomain(string) bool

	Profile() *schema.PlatformProfile

	DiscoverRepos() ([]schema.Repo, error)
	RepoHasTediumConfig(repo *schema.Repo) (bool, error)
	ReadRepoFile(repo *schema.Repo, pathCandidates []string) ([]byte, error)
	OpenOrUpdatePullRequest(job *schema.Job) error
}

func FromDomain(domain string) Platform {
	for _, platform := range platformCache {
		if platform.AcceptsDomain(domain) {
			return platform
		}
	}

	return nil
}

func FromConfig(conf *schema.TediumConfig, platformConfig *schema.PlatformConfig) (Platform, error) {
	var platform Platform

	if platformConfig.Auth == nil {
		l.Warn("Platform created without auth config; it will only be able to read public repos and will not be able to create PRs.", "domain", platformConfig.Domain)
	}

	switch platformConfig.Type {
	case "gitea":
		p, err := giteaPlatformFromConfig(conf, platformConfig)
		if err != nil {
			return nil, fmt.Errorf("Error building Gitea platform: %w", err)
		}
		platform = p

	case "github":
		p, err := githubPlatformFromConfig(conf, platformConfig)
		if err != nil {
			return nil, fmt.Errorf("Error building GitHub platform: %w", err)
		}
		platform = p
	}

	if platform != nil {
		platformCache = append(platformCache, platform)
		return platform, nil
	}

	return nil, fmt.Errorf("Unrecognised platform type: %s", platformConfig.Type)
}
