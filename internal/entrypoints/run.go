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

type RunStats struct {
	ReposDiscovered int
	ReposSkipped    int
	ReposFailed     int
	JobsDiscovered  int
	JobsFailed      int
}

var runStats = &RunStats{}

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

		runStats.JobsDiscovered++

		platform, err := platforms.FromConfig(conf, job.PlatformConfig)
		if err != nil {
			l.Error("Failed to get platform for job - aborting this chore", "error", err, "repo", job.Repo.FullName(), "chore", job.Chore.Name)
			runStats.JobsFailed++
			continue
		}

		err = executors.PrepareJob(platform, job)
		if err != nil {
			l.Error("Failed to prepare job - aborting this chore", "error", err, "repo", job.Repo.FullName(), "chore", job.Chore.Name)
			runStats.JobsFailed++
			continue
		}

		l.Info("Executing chore", "repo", job.Repo.FullName(), "chore", job.Chore.Name)
		err = executor.ExecuteChore(job)
		if err != nil {
			l.Error("Error executing chore - aborting this chore", "error", err, "repo", job.Repo.FullName(), "chore", job.Chore.Name)
			runStats.JobsFailed++
			continue
		}
	}

	l.Info("Stats", "stats", runStats)

	l.Info("Cleaning up executor")
	err = executor.Deinit()
	if err != nil {
		l.Error("Error de-initialising executor", "error", err)
		os.Exit(1)
	}
}

func gatherJobs(conf *schema.TediumConfig) *utils.Queue[schema.Job] {
	var jobQueue utils.Queue[schema.Job]

	// init ALL platforms before trying to use ANY of them
	for id := range conf.Platforms {
		platformConfig := &conf.Platforms[id]

		l.Info("Initialising platform", "domain", platformConfig.Domain)
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
	}

	for id := range conf.Platforms {
		platformConfig := &conf.Platforms[id]
		platform := platforms.FromDomain(platformConfig.Domain)
		if platform == nil {
			// this shouldn't ever happen
			l.Error("Unable to retrieve existing platform by domain", "domain", platformConfig.Domain)
			os.Exit(1)
		}

		if platformConfig.SkipDiscovery {
			continue
		}

		l.Info("Discovering repos")
		allRepos, err := platform.DiscoverRepos()
		if err != nil {
			l.Error("Error discovering repos", "error", err)
			os.Exit(1)
		}

		l.Info("Finished discovering repos", "count", len(allRepos))
		runStats.ReposDiscovered = len(allRepos)

		for targetRepoIdx := range allRepos {
			targetRepo := &allRepos[targetRepoIdx]

			if targetRepo.Archived {
				l.Info("Repo is archived - skipping", "repo", targetRepo.FullName())
				runStats.ReposSkipped++
				continue
			}

			if !platformConfig.AcceptsRepo(targetRepo.FullName()) {
				l.Info("Repo does not match any filter - skipping", "repo", targetRepo.FullName())
				runStats.ReposSkipped++
				continue
			}

			hasConfig, err := platform.RepoHasTediumConfig(targetRepo)
			if err != nil {
				l.Error("Error checking whether repo has a Tedium config", "repo", targetRepo.FullName(), "error", err)
				os.Exit(1)
			}
			if !hasConfig {
				l.Info("Repo has no Tedium config - skipping", "repo", targetRepo.FullName())
				runStats.ReposSkipped++
				continue

				// TODO: auto-enrollment
			}

			repoConfig, err := resolveRepoConfig(conf, targetRepo)
			if err != nil {
				l.Error("Error resolving repo config", "repo", targetRepo.FullName(), "error", err)
				runStats.ReposFailed++
				os.Exit(1)
			}

			l.Info("Resolved chores for repo", "repo", targetRepo.FullName(), "chores", len(repoConfig.Chores))

			for choreIdx := range repoConfig.Chores {
				chore := repoConfig.Chores[choreIdx]
				jobQueue.Push(schema.Job{
					Config:          conf,
					Repo:            targetRepo,
					Chore:           chore,
					PlatformConfig:  platformConfig,
					WorkBranchName:  utils.UniqueName("work"),
					FinalBranchName: utils.ConvertToBranchName(chore.Name),
				})
			}
		}
	}

	// de-init platforms after ALL of them are finished with
	for id := range conf.Platforms {
		platformConfig := &conf.Platforms[id]
		platform := platforms.FromDomain(platformConfig.Domain)
		if platform == nil {
			// this shouldn't ever happen
			l.Error("Unable to retrieve existing platform by domain", "domain", platformConfig.Domain)
			os.Exit(1)
		}

		l.Info("De-initialising platform", "domain", platformConfig.Domain)
		err := platform.Deinit()
		if err != nil {
			l.Error("Error de-initialising platform", "error", err)
			os.Exit(1)
		}
	}

	return &jobQueue
}
