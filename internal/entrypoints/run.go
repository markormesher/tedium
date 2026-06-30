package entrypoints

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/markormesher/tedium/internal/executor"
	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

func Run(conf schema.TediumConfig) {
	// set up queues
	jobQueue := make(chan schema.Job, conf.Executor.ChoreConcurrency*100)
	eventQueue := make(chan schema.Event, conf.Executor.ChoreConcurrency*10)

	// setup the executor
	slog.Info("initialising executor")
	err := executor.CreateAndStart(conf, jobQueue, eventQueue)
	if err != nil {
		slog.Error("could not initialise executor", "error", err)
		os.Exit(1)
	}

	// gather jobs and feed them to the executor
	slog.Info("starting to gather chores")
	go gatherJobs(conf, jobQueue, eventQueue)

	// watch events and wait for completion
	done := watchEvents(eventQueue)
	<-done
}

func watchEvents(eventQueue <-chan schema.Event) chan struct{} {
	type stats struct {
		ReposDiscovered int
		ReposSkipped    int
		ReposFailed     int
		JobsDiscovered  int
		JobsSuceeded    int
		JobsFailed      int
	}

	done := make(chan struct{})
	discoveryFinished := false
	s := stats{}

	logProgress := func() {
		slog.Info("progress", "stats", s)
	}

	// this is the only routine that modifies the state above, so no locking is needed
	go func() {
		for e := range eventQueue {
			switch e {
			case schema.RepoDiscovered:
				s.ReposDiscovered++

			case schema.RepoSkipped:
				s.ReposSkipped++

			case schema.RepoFailed:
				s.ReposFailed++

			case schema.DiscoveryFinished:
				discoveryFinished = true

			case schema.JobDiscovered:
				s.JobsDiscovered++

			case schema.JobSucceeded:
				s.JobsSuceeded++

			case schema.JobFailed:
				s.JobsFailed++
			}

			if discoveryFinished && s.JobsDiscovered == s.JobsSuceeded+s.JobsFailed {
				logProgress()
				done <- struct{}{}
			}
		}
	}()

	// regularly print stats
	go func() {
		for range time.Tick(time.Second * 10) {
			logProgress()
		}
	}()

	return done
}

func gatherJobs(conf schema.TediumConfig, jobQueue chan<- schema.Job, eventQueue chan<- schema.Event) {
	// init ALL platforms before trying to use ANY of them
	for _, platformConfig := range conf.Platforms {
		slog.Info("initialising platform", "baseURL", platformConfig.BaseURL)
		platform, err := platforms.FromConfig(conf, platformConfig)
		if err != nil {
			slog.Error("error initialising platform", "error", err)
			os.Exit(1)
		}

		err = platform.Init(conf)
		if err != nil {
			slog.Error("error initialising platform", "error", err)
			os.Exit(1)
		}
	}

	for _, platformConfig := range conf.Platforms {
		platform := platforms.FromURL(platformConfig.BaseURL)
		if platform == nil {
			// this shouldn't ever happen
			slog.Error("unable to retrieve existing platform by base URL", "baseURL", platformConfig.BaseURL)
			os.Exit(1)
		}

		if platformConfig.SkipDiscovery {
			continue
		}

		slog.Info("discovering repos")
		allRepos, err := platform.DiscoverRepos()
		if err != nil {
			slog.Error("error discovering repos", "error", err)
			os.Exit(1)
		}

		slog.Info("finished discovering repos", "count", len(allRepos))

		for _, targetRepo := range allRepos {
			eventQueue <- schema.RepoDiscovered

			if targetRepo.Archived {
				slog.Info("repo is archived - skipping", "repo", targetRepo.FullName())
				eventQueue <- schema.RepoSkipped
				continue
			}

			if targetRepo.Mirror {
				slog.Info("repo is a mirror - skipping", "repo", targetRepo.FullName())
				eventQueue <- schema.RepoSkipped
				continue
			}

			if !platformConfig.AcceptsRepo(targetRepo.FullName()) {
				slog.Info("repo does not match any filter - skipping", "repo", targetRepo.FullName())
				eventQueue <- schema.RepoSkipped
				continue
			}

			hasConfig, err := platform.RepoHasTediumConfig(targetRepo)
			if err != nil {
				slog.Error("error checking whether repo has a Tedium config", "repo", targetRepo.FullName(), "error", err)
				eventQueue <- schema.RepoFailed
				continue
			}

			if !hasConfig {
				slog.Info("repo has no Tedium config - skipping", "repo", targetRepo.FullName())
				eventQueue <- schema.RepoSkipped
				continue

				// TODO: auto-enrollment
			}

			repoConfig, err := resolveRepoConfig(conf, targetRepo)
			if err != nil {
				slog.Error("error resolving repo config", "repo", targetRepo.FullName(), "error", err)
				eventQueue <- schema.RepoFailed
				continue
			}

			slog.Info("resolved chores for repo", "repo", targetRepo.FullName(), "chores", len(repoConfig.Chores))

			for _, chore := range repoConfig.Chores {
				eventQueue <- schema.JobDiscovered

				job, err := prepareJob(conf, chore, targetRepo, platform)
				if err != nil {
					slog.Error("error preparing job", "repo", targetRepo.FullName(), "chore", chore.Name, "error", err)
					eventQueue <- schema.JobFailed
					continue
				}

				jobQueue <- job
			}
		}
	}

	// de-init platforms after ALL of them are finished with
	for _, platformConfig := range conf.Platforms {
		platform := platforms.FromURL(platformConfig.BaseURL)
		if platform == nil {
			// this shouldn't ever happen
			slog.Error("unable to retrieve existing platform by base URL", "baseURL", platformConfig.BaseURL)
			os.Exit(1)
		}

		slog.Info("de-initialising platform", "baseURL", platformConfig.BaseURL)
		err := platform.Deinit()
		if err != nil {
			slog.Error("error de-initialising platform", "error", err)
			os.Exit(1)
		}
	}

	eventQueue <- schema.DiscoveryFinished
	close(jobQueue)
}

func prepareJob(conf schema.TediumConfig, chore schema.ChoreSpec, targetRepo schema.Repo, platform platforms.Platform) (schema.Job, error) {
	job := schema.Job{
		Config:          conf,
		Repo:            targetRepo,
		Chore:           chore,
		PlatformConfig:  platform.Config(),
		WorkBranchName:  utils.UniqueName("work"),
		FinalBranchName: utils.ConvertToBranchName(chore.Name),
	}

	jobEnvBundle, err := job.ToEnvironment()
	if err != nil {
		return schema.Job{}, fmt.Errorf("error generating job environment variable: %w", err)
	}

	tediumImage := conf.Images.Tedium

	if !job.Chore.SkipCloneStep {
		tediumStep := schema.ChoreStep{
			Image:       tediumImage,
			Command:     "/usr/local/bin/tedium --internal-command initChore",
			Environment: jobEnvBundle,
			Internal:    true,
		}
		job.Chore.Steps = append([]schema.ChoreStep{tediumStep}, job.Chore.Steps...)
	}

	if !job.Chore.SkipFinaliseStep {
		tediumStep := schema.ChoreStep{
			Image:       tediumImage,
			Command:     "/usr/local/bin/tedium --internal-command finaliseChore",
			Environment: jobEnvBundle,
			Internal:    true,
		}
		job.Chore.Steps = append(job.Chore.Steps, tediumStep)
	}

	job.ExecutionSteps = make([]schema.ExecutionStep, len(job.Chore.Steps))
	for i, step := range job.Chore.Steps {
		job.ExecutionSteps[i] = schema.ExecutionStep{
			Label:       fmt.Sprintf("step-%d", i+1),
			Image:       step.Image,
			Command:     step.Command,
			Environment: envForStep(platform, job, step),
		}
	}

	return job, nil
}

func envForStep(platform platforms.Platform, job schema.Job, step schema.ChoreStep) map[string]string {
	env := map[string]string{}

	// used by Tedium directly
	env["TEDIUM_COMMAND"] = step.Command

	// not used by Tedium directly
	env["TEDIUM_REPO_OWNER"] = job.Repo.OwnerName
	env["TEDIUM_REPO_NAME"] = job.Repo.Name
	env["TEDIUM_REPO_CLONE_URL"] = job.Repo.CloneURL
	env["TEDIUM_REPO_DEFAULT_BRANCH"] = job.Repo.DefaultBranch
	env["TEDIUM_PLATFORM_TYPE"] = platform.Config().Type
	env["TEDIUM_PLATFORM_BASE_URL"] = platform.Config().BaseURL
	env["TEDIUM_PLATFORM_API_BASE_URL"] = platform.APIBaseURL().String()
	env["TEDIUM_PLATFORM_EMAIL"] = platform.Profile().Email
	if job.Chore.SourceConfig.ExposePlatformToken {
		env["TEDIUM_PLATFORM_TOKEN"] = platform.AuthToken()
	}

	for k, v := range step.Environment {
		if !step.Internal && strings.HasPrefix(k, "TEDIUM_") {
			slog.Warn("not passing environment variable to chore step", "key", k)
		} else {
			env[k] = v
		}
	}

	for k, v := range job.Chore.SourceConfig.Environment {
		if strings.HasPrefix(k, "TEDIUM_") {
			slog.Warn("not passing environment variable to chore step", "key", k)
		} else {
			env[k] = v
		}
	}

	return env
}
