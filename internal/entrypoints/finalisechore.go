package entrypoints

import (
	"os"

	"github.com/markormesher/tedium/internal/git"
	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
)

func FinaliseChore() {
	job, err := schema.JobFromEnvironment()
	if err != nil {
		l.Error("Error getting job from environment", "error", err)
		os.Exit(1)
	}

	platform, err := platforms.FromConfig(job.Config, job.PlatformConfig)
	if err != nil {
		l.Error("Error getting platform from environment", "error", err)
		os.Exit(1)
	}

	err = platform.Init(job.Config)
	if err != nil {
		l.Error("Error initialising platform", "error", err)
		os.Exit(1)
	}

	changedThisRun, err := git.CommitIfChanged(job, platform.Profile())
	if err != nil {
		l.Error("Error committing changes", "error", err)
		os.Exit(1)
	}

	if !changedThisRun {
		l.Info("Chore did not modify the repo")
		os.Exit(0)
		return
	}

	changedSincePreviousRuns, err := git.WorkBranchDiffersFromFinalBranch(job)
	if err != nil {
		l.Error("Error comparing work and final branches", "error", err)
		os.Exit(1)
	}

	if !changedSincePreviousRuns {
		l.Info("Identical changes have already been pushed, no need to overwrite them")
		os.Exit(0)
		return
	}

	err = git.PushWorkBranchToFinalBranch(job)
	if err != nil {
		l.Error("Error pushing changes", "error", err)
		os.Exit(1)
	}

	err = platform.OpenOrUpdatePullRequest(job)
	if err != nil {
		l.Error("Error opening or updating PR", "error", err)
		os.Exit(1)
	}
}
