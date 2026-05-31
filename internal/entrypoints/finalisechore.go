package entrypoints

import (
	"log/slog"
	"os"

	"github.com/markormesher/tedium/internal/git"
	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
)

func FinaliseChore() {
	job, err := schema.JobFromEnvironment()
	if err != nil {
		slog.Error("Error getting job from environment", "error", err)
		os.Exit(1)
	}

	platform, err := platforms.FromConfig(job.Config, job.PlatformConfig)
	if err != nil {
		slog.Error("Error getting platform from environment", "error", err)
		os.Exit(1)
	}

	err = platform.Init(job.Config)
	if err != nil {
		slog.Error("Error initialising platform", "error", err)
		os.Exit(1)
	}

	changedThisRun, err := git.CommitIfChanged(job, platform.Profile())
	if err != nil {
		slog.Error("Error committing changes", "error", err)
		os.Exit(1)
	}

	if !changedThisRun {
		slog.Info("Chore did not modify the repo")
		os.Exit(0)
		return
	}

	changedSincePreviousRuns, err := git.WorkBranchDiffersFromFinalBranch(job)
	if err != nil {
		slog.Error("Error comparing work and final branches", "error", err)
		os.Exit(1)
	}

	if !changedSincePreviousRuns {
		slog.Info("Identical changes have already been pushed, no need to overwrite them")
		os.Exit(0)
		return
	}

	err = git.PushWorkBranchToFinalBranch(job)
	if err != nil {
		slog.Error("Error pushing changes", "error", err)
		os.Exit(1)
	}

	err = platform.OpenOrUpdatePullRequest(job)
	if err != nil {
		slog.Error("Error opening or updating PR", "error", err)
		os.Exit(1)
	}
}
