package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"

	"github.com/markormesher/tedium/internal/logging"
)

var l = logging.Logger

// TediumConfig is passed to the Tedium executable to control its behaviour.
type TediumConfig struct {
	// Images defines the container images used for Tedium-owned stages of execution
	Images struct {
		Tedium string `json:"tedium"`
		Pause  string `json:"pause"`
	} `json:"images"`

	// RepoStoragePath defines the path on disk where repos should be cloned when needed locally. If blank a temporary folder will be created.
	RepoStoragePath               string `json:"repoStoragePath"`
	RepoStoragePathWasAutoCreated bool

	// Platforms defines the set of repository hosting platforms that repos will be discovered from.
	Platforms map[string]*PlatformConfig `json:"platforms"`

	// Auth defines additional authentication details. The keys are expected to be domains.
	Auth map[string]*AuthConfig `json:"auth"`

	// Executor defines the actual executor that will be used to perform chores.
	Executor ExecutorConfig `json:"executor"`

	// AutoEnrollment defines the Tedium config to apply to repos that don't already have one.
	AutoEnrollment struct {
		Enabled bool       `json:"enabled"`
		Config  RepoConfig `json:"config"`
	} `json:"autoEnrollment"`
}

// ---

func LoadTediumConfig(filePath string) (*TediumConfig, error) {
	configFileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading configuration file: %v", err)
	}

	var conf TediumConfig
	decoder := json.NewDecoder(bytes.NewReader(configFileContent))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&conf)
	if err != nil {
		return nil, fmt.Errorf("Error parsing configuration file: %v", err)
	}

	err = conf.CompileRepoFilters()
	if err != nil {
		return nil, fmt.Errorf("Error compiling repo filters in configuration: %v", err)
	}

	if conf.RepoStoragePath == "" {
		conf.RepoStoragePathWasAutoCreated = true
		conf.RepoStoragePath, err = os.MkdirTemp("", "tedium")
		if err != nil {
			return nil, fmt.Errorf("Failed to create a temporary directory for repo storage: %v", err)
		}
	}

	// apply defaults

	if conf.Images.Pause == "" {
		conf.Images.Pause = "ghcr.io/markormesher/tedium-pause:latest"
	}

	return &conf, nil
}

func (conf *TediumConfig) GetAuthConfig(endpointOrCloneUrl string) *AuthConfig {
	// preference 1: take the auth from platform config, looking for exact matches on endpoint URL

	for i := range conf.Platforms {
		if endpointOrCloneUrl == conf.Platforms[i].Endpoint {
			auth := conf.Platforms[i].Auth
			if auth != nil {
				return auth
			}
		}
	}

	// preference 2: take the auth from config.Auth, looking for matches on domain

	parsed, err := url.Parse(endpointOrCloneUrl)
	if err != nil {
		l.Warn("Failed to parse domain from endpoint or clone URL - no auth will be used", "endpointOrCloneUrl", endpointOrCloneUrl)
		return nil
	}
	domain := parsed.Hostname()

	if conf.Auth[domain] != nil {
		return conf.Auth[domain]
	}

	// preference 3: go back to checking platforms for auth, matching on domain only

	for i := range conf.Platforms {
		parsed, err = url.Parse(conf.Platforms[i].Endpoint)
		if err != nil {
			l.Warn("Failed to parse domain from platform endpoint - it will not be used as an auth source", "endpoint", conf.Platforms[i].Endpoint)
			continue
		}

		if domain == parsed.Hostname() {
			auth := conf.Platforms[i].Auth
			if auth != nil {
				return auth
			}
		}
	}

	// give up

	return nil
}

func (conf TediumConfig) CompileRepoFilters() error {
	for i := range conf.Platforms {
		p := conf.Platforms[i]
		if p.RepoFiltersRaw == nil || len(p.RepoFiltersRaw) == 0 {
			p.RepoFilters = nil
			continue
		}

		p.RepoFilters = make([]*regexp.Regexp, len(p.RepoFiltersRaw))
		for fi, f := range conf.Platforms[i].RepoFiltersRaw {
			r, err := regexp.Compile(f)
			if err != nil {
				return fmt.Errorf("Error compiling repo filter regex: %w", err)
			}

			p.RepoFilters[fi] = r
		}
	}

	return nil
}
