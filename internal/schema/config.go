package schema

// NOTE: this file is referenced in the README - update any links if you move or rename this file.

import (
	"bytes"
	"fmt"
	"os"
	"regexp"

	"github.com/markormesher/tedium/internal/utils"
	"gopkg.in/yaml.v3"
)

// TediumConfig is passed to the Tedium executable to control its behaviour.
type TediumConfig struct {
	// Executor defines the actual executor that will be used to perform chores.
	Executor ExecutorConfig `json:"executor" yaml:"executor"`

	// Platforms defines the set of repository hosting platforms that repos will be discovered from.
	Platforms []PlatformConfig `json:"platforms" yaml:"platforms"`

	// Images defines the container images used for Tedium-owned stages of execution
	Images struct {
		Tedium string `json:"tedium" yaml:"tedium"`
		Pause  string `json:"pause" yaml:"pause"`
	} `json:"images" yaml:"images"`

	// AutoEnrollment defines the Tedium config to apply to repos that don't already have one.
	AutoEnrollment struct {
		Enabled bool       `json:"enabled" yaml:"enabled"`
		Config  RepoConfig `json:"config" yaml:"config"`
	} `json:"autoEnrollment" yaml:"autoEnrollment"`

	// ChoreConcurrency defines how many chores Tedium should attempt to run concurrently. It is an upper bound and may not be reached. Defaults to 1.
	ChoreConcurrency int `json:"choreConcurrency" yaml:"choreConcurrency"`
}

// RepoConfig is read from a target repo. The main purpose is to define which chores are to be applied.
type RepoConfig struct {
	Extends []string          `json:"extends,omitempty" yaml:"extends,omitempty"`
	Chores  []RepoChoreConfig `json:"chores,omitempty" yaml:"chores,omitempty"`
}

// RepoChoreConfig defines one chore to apply to a repo.
type RepoChoreConfig struct {
	Url       string `json:"url" yaml:"url"`
	Directory string `json:"directory" yaml:"directory"`

	// Branch specifies the bracnh to read the chore definition from. If blank the default branch will be used.
	Branch string `json:"branch" yaml:"branch"`

	// Environment specifies additional environment variables to be passed to all stages of chore execution. Variables must not start with "TEDIUM_.
	Environment map[string]string `json:"environment" yaml:"environment"`

	// ExposePlatformToken specifies that the target repo's platform auth token should be exposed to chore steps via the TEDIUM_PLATFORM_TOKEN environment variable. Use with caution.
	ExposePlatformToken bool `json:"exposePlatformToken" yaml:"exposePlatformToken"`
}

// ResolvedRepoConfig is the result of taking a target repo, following all "extends" links, and resolving all chore references into their actual spec.
type ResolvedRepoConfig struct {
	Chores []ChoreSpec
}

// ---

func LoadTediumConfig(configFilePath string) (TediumConfig, error) {
	configFileContent, err := os.ReadFile(configFilePath)
	if err != nil {
		return TediumConfig{}, fmt.Errorf("error reading configuration file: %v", err)
	}

	var conf TediumConfig
	if utils.IsYamlOrJsonFile(configFilePath) {
		decoder := yaml.NewDecoder(bytes.NewReader(configFileContent))
		decoder.KnownFields(true)
		err := decoder.Decode(&conf)
		if err != nil {
			return TediumConfig{}, fmt.Errorf("error parsing configuration file: %v", err)
		}
	} else {
		return TediumConfig{}, fmt.Errorf("unacceptable file format: %s", configFilePath)
	}

	err = conf.CompileRepoFilters()
	if err != nil {
		return TediumConfig{}, fmt.Errorf("error compiling repo filters in configuration: %v", err)
	}

	// apply defaults

	if conf.Images.Pause == "" {
		conf.Images.Pause = "ghcr.io/markormesher/tedium-pause:v0"
	}

	if conf.Images.Tedium == "" {
		conf.Images.Tedium = "ghcr.io/markormesher/tedium:v0"
	}

	if conf.ChoreConcurrency < 1 {
		conf.ChoreConcurrency = 1
	}

	// sanity checks

	if conf.Executor.Podman != nil && conf.Executor.Kubernetes != nil {
		return TediumConfig{}, fmt.Errorf("invalid Tedium config: more than one executor configured")
	}

	domainsSeen := make(map[string]bool)
	for _, platform := range conf.Platforms {
		domain := platform.Domain
		if domainsSeen[domain] {
			return TediumConfig{}, fmt.Errorf("invalid Tedium config: duplicate platform domain %s", domain)
		}
		domainsSeen[domain] = true
	}

	return conf, nil
}

func (conf *TediumConfig) CompileRepoFilters() error {
	for _, p := range conf.Platforms {
		if len(p.RepoFiltersRaw) == 0 {
			p.RepoFilters = nil
			continue
		}

		p.RepoFilters = make([]*regexp.Regexp, len(p.RepoFiltersRaw))
		for fi, f := range p.RepoFiltersRaw {
			r, err := regexp.Compile(f)
			if err != nil {
				return fmt.Errorf("error compiling repo filter regex: %w", err)
			}

			p.RepoFilters[fi] = r
		}
	}

	return nil
}
