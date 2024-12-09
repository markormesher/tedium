package entrypoints

import (
	"os"

	"github.com/markormesher/tedium/internal/git"
	"github.com/markormesher/tedium/internal/schema"
)

func InitChore() {
	job, err := schema.JobFromEnvironment()
	if err != nil {
		l.Error("Error getting job from environment", "error", err)
		os.Exit(1)
	}

	err = git.CloneRepo(job, job.Config)
	if err != nil {
		l.Error("Error cloning repo", "error", err)
		os.Exit(1)
	}

	err = git.CheckoutWorkBranch(job)
	if err != nil {
		l.Error("Error checking out branch for chore", "error", err)
		os.Exit(1)
	}
}
