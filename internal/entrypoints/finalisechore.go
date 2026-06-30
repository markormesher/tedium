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
		slog.Error("error getting job from environment", "error", err)
		os.Exit(1)
	}

	platform, err := platforms.FromConfig(job.Config, job.PlatformConfig)
	if err != nil {
		slog.Error("error getting platform from environment", "error", err)
		os.Exit(1)
	}

	err = platform.Init(job.Config)
	if err != nil {
		slog.Error("error initialising platform", "error", err)
		os.Exit(1)
	}

	changedThisRun, err := git.CommitIfChanged(job, platform.Profile())
	if err != nil {
		slog.Error("error committing changes", "error", err)
		os.Exit(1)
	}

	if !changedThisRun {
		slog.Info("chore did not modify the repo")
		os.Exit(0)
		return
	}

	changedSincePreviousRuns, err := git.WorkBranchDiffersFromFinalBranch(job)
	if err != nil {
		slog.Error("error comparing work and final branches", "error", err)
		os.Exit(1)
	}

	if !changedSincePreviousRuns {
		slog.Info("identical changes have already been pushed, no need to overwrite them")
		os.Exit(0)
		return
	}

	err = git.PushWorkBranchToFinalBranch(job)
	if err != nil {
		slog.Error("error pushing changes", "error", err)
		os.Exit(1)
	}

	err = platform.OpenOrUpdatePullRequest(job)
	if err != nil {
		slog.Error("error opening or updating PR", "error", err)
		os.Exit(1)
	}
}
