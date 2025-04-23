package entrypoints

import (
	"bytes"
	"fmt"

	"maps"

	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
	"gopkg.in/yaml.v3"
)

func resolveRepoConfig(_ schema.TediumConfig, targetRepo schema.Repo) (schema.ResolvedRepoConfig, error) {
	// approach:
	// - starting from the target repo, recursively follow "extends" urls
	// - build a LIFO stack of configs to apply, ending with the target repo
	// - initalise a blank config, then merge it with every element in the stack
	// - with the merged repo config, inflate every chore into the full spec

	// collect configs to merge later
	var configsToMerge utils.Stack[schema.RepoConfig]

	// non-looping depth-first search on "extends" urls
	urlsVisited := map[string]bool{}
	var urlsToVisit utils.Queue[string]
	urlsToVisit.Push(targetRepo.CloneUrl)
	for {
		configUrl, ok := urlsToVisit.Pop()
		if !ok {
			break
		}
		urlsVisited[configUrl] = true

		var fileName string
		if configsToMerge.Size == 0 {
			// this is the target repo, not an extended config
			fileName = ".tedium"
		} else {
			fileName = "index"
		}

		configRepo, err := schema.RepoFromUrl(configUrl)
		if err != nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("error constructing config repo before reading its config: %w", err)
		}

		platform := platforms.FromDomain(configRepo.Domain)
		if platform == nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to determine a platform to read repo config (domain: %s)", configRepo.Domain)
		}

		var repoConfigRaw []byte
		repoConfigRaw, err = platform.ReadRepoFile(configRepo, "", utils.AddYamlJsonExtensions(fileName))
		if err != nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to read config file out of repo: %w", err)
		}
		if len(repoConfigRaw) == 0 {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to read config file out of repo: no file exists")
		}

		var repoConfig *schema.RepoConfig
		decoder := yaml.NewDecoder(bytes.NewReader(repoConfigRaw))
		decoder.KnownFields(true)
		err = decoder.Decode(&repoConfig)
		if err != nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to unmarshal repo config file: %w", err)
		}

		for _, extendsUrl := range repoConfig.Extends {
			visited := urlsVisited[extendsUrl]
			if visited {
				l.Warn("loop detected in config extension - saw a URL for the second time", "url", extendsUrl)
			} else {
				urlsToVisit.Push(extendsUrl)
			}
		}

		configsToMerge.Push(*repoConfig)
	}

	// merge all configs on top of a blank template
	var mergedConfig schema.RepoConfig
	for {
		config, ok := configsToMerge.Pop()
		if !ok {
			break
		}

		var err error
		mergedConfig, err = mergeRepoConfigs(mergedConfig, config)
		if err != nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("error merging configs: %w", err)
		}
	}

	// for every chore in the merged config, resolve it into the actual chore spec
	resolvedConfig := schema.ResolvedRepoConfig{
		Chores: make([]schema.ChoreSpec, len(mergedConfig.Chores)),
	}
	for souceChoreIdx, sourceChore := range mergedConfig.Chores {
		choreRepoUrl := sourceChore.Url
		choreBranch := sourceChore.Branch
		choreDirectory := sourceChore.Directory

		choreRepo, err := schema.RepoFromUrl(choreRepoUrl)
		if err != nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("error constructing chore repo before reading its config: %w", err)
		}

		platform := platforms.FromDomain(choreRepo.Domain)
		if platform == nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to determine a platform to read chore config (domain: %s)", choreRepo.Domain)
		}

		var choreSpecRaw []byte
		choreSpecRaw, err = platform.ReadRepoFile(choreRepo, choreBranch, utils.AddYamlJsonExtensions(fmt.Sprintf("%s/chore", choreDirectory)))
		if err != nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to read chore file out of repo: %w", err)
		}
		if choreSpecRaw == nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to read chore file out of repo: no file exists")
		}

		var choreSpec schema.ChoreSpec
		decoder := yaml.NewDecoder(bytes.NewReader(choreSpecRaw))
		decoder.KnownFields(true)
		err = decoder.Decode(&choreSpec)
		if err != nil {
			return schema.ResolvedRepoConfig{}, fmt.Errorf("failed to unmarshal chore config file: %w", err)
		}

		choreSpec.SourceConfig = sourceChore

		resolvedConfig.Chores[souceChoreIdx] = choreSpec
	}

	return resolvedConfig, nil
}

func mergeRepoConfigs(a, b schema.RepoConfig) (schema.RepoConfig, error) {
	// merging rules:
	// - don't copy "extends" URLs, because this happens after they have been explored
	// - copy all chores from A
	// - for each chore in B,	if it was already defined on A then merge them, otherwise append

	merged := schema.RepoConfig{}

	// populate chores from A
	merged.Chores = append(merged.Chores, a.Chores...)

	// merge in chores from B
	for _, c := range b.Chores {
		// if this chore is already defined, overwrite it with a merged version
		didOverwrite := false
		for i, cm := range merged.Chores {
			if c.Url == cm.Url && c.Directory == cm.Directory {
				mergedChore, err := mergeChoreConfigs(cm, c)
				if err != nil {
					return schema.RepoConfig{}, err
				}

				merged.Chores[i] = mergedChore
				didOverwrite = true
				break
			}
		}

		if !didOverwrite {
			merged.Chores = append(merged.Chores, c)
		}
	}

	return merged, nil
}

func mergeChoreConfigs(a, b schema.RepoChoreConfig) (schema.RepoChoreConfig, error) {
	merged := a

	if b.ExposePlatformToken {
		merged.ExposePlatformToken = true
	}

	if b.Branch != "" {
		merged.Branch = b.Branch
	}

	if b.Environment != nil {
		if merged.Environment == nil {
			merged.Environment = b.Environment
		} else {
			maps.Copy(merged.Environment, b.Environment)
		}
	}

	return merged, nil
}
