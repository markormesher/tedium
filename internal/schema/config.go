package schema

// NOTE: this file is referenced in the README - update any links if you move or rename this file.

import (
	"bytes"
	"fmt"
	"os"
	"regexp"

	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/utils"
	"gopkg.in/yaml.v3"
)

var l = logging.Logger

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
}

// RepoConfig is read from a target repo. The main purpose is to define which chores are to be applied.
type RepoConfig struct {
	Extends []string          `json:"extends,omitempty" yaml:"extends,omitempty"`
	Chores  []RepoChoreConfig `json:"chores,omitempty" yaml:"chores,omitempty"`
}

// RepoChoreConfig defines one chore to apply to a repo.
type RepoChoreConfig struct {
	RepoUrl   string `json:"repoUrl" yaml:"repoUrl"`
	Directory string `json:"directory" yaml:"directory"`

	// Branch specifies the bracnh to read the chore definition from. If blank the default branch will be used.
	Branch string `json:"branch" yaml:"branch"`

	// Environment specifies additional environment variables to be passed to all stages of chore execution. Variables must not start with "TEDIUM_.
	Environment map[string]string `json:"environment" yaml:"environment"`
}

// ResolvedRepoConfig is the result of taking a target repo, following all "extends" links, and resolving all chore references into their actual spec.
type ResolvedRepoConfig struct {
	Chores []*ChoreSpec
}

// ---

func LoadTediumConfig(configFilePath string) (*TediumConfig, error) {
	configFileContent, err := os.ReadFile(configFilePath)
	if err != nil {
		return nil, fmt.Errorf("Error reading configuration file: %v", err)
	}

	var conf TediumConfig
	if utils.IsYamlOrJsonFile(configFilePath) {
		decoder := yaml.NewDecoder(bytes.NewReader(configFileContent))
		decoder.KnownFields(true)
		err := decoder.Decode(&conf)
		if err != nil {
			return nil, fmt.Errorf("Error parsing configuration file: %v", err)
		}
	} else {
		return nil, fmt.Errorf("Unacceptable file format: %s", configFilePath)
	}

	err = conf.CompileRepoFilters()
	if err != nil {
		return nil, fmt.Errorf("Error compiling repo filters in configuration: %v", err)
	}

	// apply defaults

	if conf.Images.Pause == "" {
		conf.Images.Pause = "ghcr.io/markormesher/tedium-pause:latest"
	}

	if conf.Images.Tedium == "" {
		conf.Images.Pause = "ghcr.io/markormesher/tedium:latest"
	}

	return &conf, nil
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
