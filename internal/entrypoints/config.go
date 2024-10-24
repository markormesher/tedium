package entrypoints

import (
	"bytes"
	"fmt"

	"github.com/markormesher/tedium/internal/git"
	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
	"gopkg.in/yaml.v3"
)

func resolveRepoConfig(conf *schema.TediumConfig, targetRepo *schema.Repo, platform platforms.Platform) (*schema.ResolvedRepoConfig, error) {
	hasConfig, err := platform.RepoHasTediumConfig(targetRepo)
	if err != nil {
		return nil, err
	}
	if !hasConfig {
		return nil, nil
	}

	// we start from a nil config, merge the root config, then merge every "extends" config on top
	// TODO: this is backwards - we need to merge the extends first, then merge/apply overrides
	var mergedConfig *schema.RepoConfig

	// frontier queue + visited set = non-looping depth-first search
	urlsVisited := make(map[string]bool)
	var urlsToVisit utils.Queue[string]
	urlsToVisit.Push(targetRepo.CloneUrl)

	for {
		url, ok := urlsToVisit.Pop()
		if !ok {
			break
		}

		urlsVisited[*url] = true

		var configContents []byte

		if *url == targetRepo.CloneUrl {
			// it's the target repo, so we can read the config from there without instantiation
			configContents, err = platform.ReadRepoFile(targetRepo, utils.AddYamlJsonExtensions(".tedium"))
			if err != nil {
				return nil, fmt.Errorf("Failed to read config file out of repo: %w", err)
			}
		} else {
			// it's not the target repo, so we need to instantiate the repo and read the config from disk
			configRepo := &schema.Repo{
				CloneUrl:   *url,
				AuthConfig: conf.GetAuthConfigForClone(*url),
			}
			err := git.CloneAndUpdateRepo(configRepo, conf)
			if err != nil {
				return nil, fmt.Errorf("Failed to instantiate repo: %w", err)
			}

			configContents, err = git.ReadFile(configRepo, utils.AddYamlJsonExtensions("index"))
			if err != nil {
				return nil, fmt.Errorf("Failed to read config file out of repo: %w", err)
			}
		}

		var repoConfig *schema.RepoConfig
		decoder := yaml.NewDecoder(bytes.NewReader(configContents))
		decoder.KnownFields(true)
		err := decoder.Decode(&repoConfig)
		if err != nil {
			return nil, fmt.Errorf("Failed to unmarshal repo config file: %w", err)
		}

		for _, extendsUrl := range repoConfig.Extends {
			visited := urlsVisited[extendsUrl]
			if visited {
				l.Warn("Loop detected in config extension - saw a URL for the second time", "url", extendsUrl)
			} else {
				urlsToVisit.Push(extendsUrl)
			}
		}

		mergedConfig, err = mergeRepoConfigs(mergedConfig, repoConfig)
		if err != nil {
			return nil, fmt.Errorf("Failed to merge using upstream config from %s: %w", url, err)
		}
	}

	// for every chore in the merged config, resolve it into the actual chore spec
	resolvedConfig := &schema.ResolvedRepoConfig{
		// TODO: copy over other parts of repo config besides chores
		Chores: make([]*schema.ChoreSpec, len(mergedConfig.Chores)),
	}
	for choreIdx := range mergedConfig.Chores {
		url := mergedConfig.Chores[choreIdx].CloneUrl
		choreRepo := &schema.Repo{
			CloneUrl:   url,
			AuthConfig: conf.GetAuthConfigForClone(url),
		}
		err := git.CloneAndUpdateRepo(choreRepo, conf)
		if err != nil {
			return nil, err
		}

		choreSpec, err := readChoreSpec(choreRepo, &mergedConfig.Chores[choreIdx])
		if err != nil {
			return nil, err
		}
		resolvedConfig.Chores[choreIdx] = choreSpec
	}

	return resolvedConfig, nil
}

func mergeRepoConfigs(a, b *schema.RepoConfig) (*schema.RepoConfig, error) {
	// deal with any nil configs up-front - after this we know references are safe
	switch {
	case a == nil && b == nil:
		return nil, fmt.Errorf("Cannot merge two nil configs")
	case a == nil:
		return b, nil
	case b == nil:
		return a, nil
	}

	// naive merge for now: just concat the chore lists and deliberately ignore the extends list
	chores := make([]schema.RepoChoreConfig, 0)
	if a.Chores != nil {
		chores = append(chores, a.Chores...)
	}
	if b.Chores != nil {
		chores = append(chores, b.Chores...)
	}

	return &schema.RepoConfig{
		Chores: chores,
	}, nil
}

func readChoreSpec(choreRepo *schema.Repo, choreConfig *schema.RepoChoreConfig) (*schema.ChoreSpec, error) {
	choreSpecFile, err := git.ReadFile(choreRepo, utils.AddYamlJsonExtensions((fmt.Sprintf("%s/chore", choreConfig.Directory))))
	if err != nil {
		return nil, fmt.Errorf("Error reading chore spec file: %v", err)
	}

	var choreSpec schema.ChoreSpec
	decoder := yaml.NewDecoder(bytes.NewReader(choreSpecFile))
	decoder.KnownFields(true)
	err = decoder.Decode(&choreSpec)
	if err != nil {
		return nil, err
	}

	return &choreSpec, nil
}
