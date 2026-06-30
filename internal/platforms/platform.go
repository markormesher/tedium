package platforms

import (
	"fmt"
	"log/slog"
	urllib "net/url"

	"github.com/markormesher/tedium/internal/schema"
)

var platformCache []Platform

type Platform interface {
	Init(conf schema.TediumConfig) error
	Deinit() error
	Config() schema.PlatformConfig
	APIBaseURL() *urllib.URL

	AcceptsURL(string) (string, bool)

	Profile() schema.PlatformProfile
	AuthToken() string

	DiscoverRepos() ([]schema.Repo, error)
	RepoHasTediumConfig(repo schema.Repo) (bool, error)
	ReadRepoFile(repo schema.Repo, branch string, pathCandidates []string) ([]byte, error)
	OpenOrUpdatePullRequest(job schema.Job) error
}

func FromURL(url string) Platform {
	for _, platform := range platformCache {
		if _, accepted := platform.AcceptsURL(url); accepted {
			return platform
		}
	}

	return nil
}

func FromConfig(conf schema.TediumConfig, platformConfig schema.PlatformConfig) (Platform, error) {
	var platform Platform

	// try the cache first
	platformFromDomain := FromURL(platformConfig.BaseURL)
	if platformFromDomain != nil {
		return platformFromDomain, nil
	}

	if platformConfig.Auth == nil {
		slog.Warn("platform created without auth config; it will only be able to read public repos and will not be able to create PRs", "baseURL", platformConfig.BaseURL)
	}

	switch platformConfig.Type {
	case "gitea":
		p, err := giteaPlatformFromConfig(platformConfig)
		if err != nil {
			return nil, fmt.Errorf("error building Gitea platform: %w", err)
		}
		platform = p

	case "github":
		p, err := githubPlatformFromConfig(platformConfig)
		if err != nil {
			return nil, fmt.Errorf("error building GitHub platform: %w", err)
		}
		platform = p
	}

	if platform != nil {
		platformCache = append(platformCache, platform)
		return platform, nil
	}

	return nil, fmt.Errorf("unrecognised platform type: %s", platformConfig.Type)
}
