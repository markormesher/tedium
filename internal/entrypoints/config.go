package entrypoints

import (
	"bytes"
	"fmt"

	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
	"gopkg.in/yaml.v3"
)

func resolveRepoConfig(conf *schema.TediumConfig, targetRepo *schema.Repo) (*schema.ResolvedRepoConfig, error) {

	// we start from a nil config, merge the root config, then merge every "extends" config on top
	// TODO: this is backwards - we need to merge the extends first, then merge/apply overrides
	var mergedConfig *schema.RepoConfig

	// frontier queue + visited set = non-looping depth-first search
	urlsVisited := make(map[string]bool)
	var urlsToVisit utils.Queue[string]
	urlsToVisit.Push(targetRepo.CloneUrl)

	visitingTargetRepo := true

	for {
		configUrl, ok := urlsToVisit.Pop()
		if !ok {
			break
		}

		urlsVisited[*configUrl] = true

		configRepo, err := schema.RepoFromUrl(*configUrl)
		if err != nil {
			return nil, fmt.Errorf("error constructing config repo before reading its config: %w", err)
		}

		platform := platforms.FromDomain(configRepo.Domain)
		if platform == nil {
			return nil, fmt.Errorf("failed to determine a platform to read repo config (domain: %s)", configRepo.Domain)
		}

		var fileName string
		if visitingTargetRepo {
			fileName = ".tedium"
		} else {
			fileName = "index"
		}

		var repoConfigRaw []byte
		repoConfigRaw, err = platform.ReadRepoFile(targetRepo, "", utils.AddYamlJsonExtensions(fileName))
		if err != nil {
			return nil, fmt.Errorf("failed to read config file out of repo: %w", err)
		}

		var repoConfig *schema.RepoConfig
		decoder := yaml.NewDecoder(bytes.NewReader(repoConfigRaw))
		decoder.KnownFields(true)
		err = decoder.Decode(&repoConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal repo config file: %w", err)
		}

		for _, extendsUrl := range repoConfig.Extends {
			visited := urlsVisited[extendsUrl]
			if visited {
				l.Warn("loop detected in config extension - saw a URL for the second time", "url", extendsUrl)
			} else {
				urlsToVisit.Push(extendsUrl)
			}
		}

		mergedConfig, err = mergeRepoConfigs(mergedConfig, repoConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to merge using upstream config from %s: %w", *configUrl, err)
		}

		visitingTargetRepo = false
	}

	// for every chore in the merged config, resolve it into the actual chore spec
	resolvedConfig := &schema.ResolvedRepoConfig{
		// TODO: copy over other parts of repo config besides chores
		Chores: make([]*schema.ChoreSpec, len(mergedConfig.Chores)),
	}
	for choreIdx := range mergedConfig.Chores {
		choreRepoUrl := mergedConfig.Chores[choreIdx].Url
		choreDirectory := mergedConfig.Chores[choreIdx].Directory

		choreRepo, err := schema.RepoFromUrl(choreRepoUrl)
		if err != nil {
			return nil, fmt.Errorf("error constructing chore repo before reading its config: %w", err)
		}

		platform := platforms.FromDomain(choreRepo.Domain)
		if platform == nil {
			return nil, fmt.Errorf("failed to determine a platform to read chore config (domain: %s)", choreRepo.Domain)
		}

		var choreSpecRaw []byte
		choreSpecRaw, err = platform.ReadRepoFile(choreRepo, "", utils.AddYamlJsonExtensions(fmt.Sprintf("%s/chore", choreDirectory)))
		if err != nil {
			return nil, fmt.Errorf("failed to read chore file out of repo: %w", err)
		}

		var choreSpec schema.ChoreSpec
		decoder := yaml.NewDecoder(bytes.NewReader(choreSpecRaw))
		decoder.KnownFields(true)
		err = decoder.Decode(&choreSpec)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal chore config file: %w", err)
		}

		resolvedConfig.Chores[choreIdx] = &choreSpec
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
