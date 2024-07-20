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

	platform, err := platforms.FromConfig(job.Config, job.PlatformEndpoint)
	if err != nil {
		l.Error("Error getting platform from environment", "error", err)
		os.Exit(1)
	}

	err = platform.Init(job.Config)
	if err != nil {
		l.Error("Error initialising platform", "error", err)
		os.Exit(1)
	}

	changed, err := git.CommitAndPushIfChanged(job, platform.BotProfile())
	if err != nil {
		l.Error("Error committing changes", "error", err)
		os.Exit(1)
	}

	if changed {
		err := platform.OpenOrUpdatePullRequest(job)
		if err != nil {
			l.Error("Error opening or updating PR", "error", err)
			os.Exit(1)
		}
	}
}
