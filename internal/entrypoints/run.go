package entrypoints

import (
	"os"

	"github.com/markormesher/tedium/internal/executors"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

var l = logging.Logger

func Run(conf *schema.TediumConfig) {
	l.Info("Starting to gather chores to do")
	jobQueue := gatherJobs(conf)
	l.Info("Finished gathering chores to do", "count", jobQueue.Size)

	if jobQueue.Size == 0 {
		l.Info("No chores to do - exiting")
		return
	}

	l.Info("Initialising executor")
	executor, err := executors.FromExecutorConfig(&conf.Executor)
	if err != nil {
		l.Error("Could not initialise executor", "error", err)
		os.Exit(1)
	}

	err = executor.Init(conf)
	if err != nil {
		l.Error("Could not initialise executor", "error", err)
		os.Exit(1)
	}

	for {
		job, ok := jobQueue.Pop()
		if !ok {
			break
		}

		err = executors.PrepareJob(job)
		if err != nil {
			l.Error("Failed to prepare job - aborting this chore", "error", err, "repo", job.Repo.FullName(), "chore", job.Chore.Name)
			// TODO: count - did some chores fail?
			continue
		}

		l.Info("Executing chore", "repo", job.Repo.FullName(), "chore", job.Chore.Name)
		err = executor.ExecuteChore(job)
		if err != nil {
			l.Error("Error executing chore - aborting this chore", "error", err, "repo", job.Repo.FullName(), "chore", job.Chore.Name)
			// TODO: count - did some chores fail?
			continue
		}
	}

	l.Info("Cleaning up executor")
	err = executor.Deinit()
	if err != nil {
		l.Error("Error de-initialising executor", "error", err)
		os.Exit(1)
	}

	if conf.RepoStoragePathWasAutoCreated {
		l.Info("Cleaning up temporary storage")
		err := os.RemoveAll(conf.RepoStoragePath)
		if err != nil {
			l.Error("Error cleaning up storage", "error", err)
			os.Exit(1)
		}
	}
}

func gatherJobs(conf *schema.TediumConfig) *utils.Queue[schema.Job] {
	var jobQueue utils.Queue[schema.Job]

	for id := range conf.Platforms {
		platformConfig := &conf.Platforms[id]

		l.Info("Initialising platform", "endpoint", platformConfig.Endpoint)
		platform, err := platforms.FromConfig(conf, platformConfig)
		if err != nil {
			l.Error("Error initialising platform", "error", err)
			os.Exit(1)
		}

		err = platform.Init(conf)
		if err != nil {
			l.Error("Error initialising platform", "error", err)
			os.Exit(1)
		}

		l.Info("Discovering repos")
		allRepos, err := platform.DiscoverRepos()
		if err != nil {
			l.Error("Error discovering repos", "error", err)
			os.Exit(1)
		}

		l.Info("Finished discovering repos", "count", len(allRepos))

		for targetRepoIdx := range allRepos {
			targetRepo := &allRepos[targetRepoIdx]

			if targetRepo.Archived {
				l.Info("Repo is archived - skipping", "repo", targetRepo.FullName())
				continue
			}

			if !platformConfig.AcceptsRepo(targetRepo.FullName()) {
				l.Info("Repo does not match any filter - skipping", "repo", targetRepo.FullName())
				continue
			}

			repoConfig, err := resolveRepoConfig(conf, targetRepo, platform)
			if err != nil {
				l.Error("Error resolving repo config", "repo", targetRepo.FullName(), "error", err)
				os.Exit(1)
			}
			if repoConfig == nil {
				if conf.AutoEnrollment.Enabled {
					// TODO: auto enrollment
					// l.Info("Repo has no Tedium config - configuring auto-enrollment")
					continue
				} else {
					l.Info("Repo has no Tedium config - skipping", "repo", targetRepo.FullName())
					continue
				}
			}

			l.Info("Resolved chores for repo", "repo", targetRepo.FullName(), "chores", len(repoConfig.Chores))

			for choreIdx := range repoConfig.Chores {
				jobQueue.Push(schema.Job{
					Config:         conf,
					Repo:           targetRepo,
					RepoConfig:     repoConfig,
					Chore:          repoConfig.Chores[choreIdx],
					PlatformConfig: platformConfig,
				})
			}
		}

		l.Info("De-initialising platform", "platform", platformConfig.Endpoint)
		err = platform.Deinit()
		if err != nil {
			l.Error("Error de-initialising platform", "error", err)
			os.Exit(1)
		}
	}

	return &jobQueue
}
