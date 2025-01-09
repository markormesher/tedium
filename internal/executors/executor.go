package executors

import (
	"fmt"
	"strings"

	"github.com/markormesher/tedium/internal/executors/kubernetes"
	"github.com/markormesher/tedium/internal/executors/podman"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/platforms"
	"github.com/markormesher/tedium/internal/schema"
)

var l = logging.Logger

func FromExecutorConfig(ec schema.ExecutorConfig) (schema.Executor, error) {
	switch {
	case ec.Podman != nil:
		return podman.FromConfig(*ec.Podman)

	case ec.Kubernetes != nil:
		return kubernetes.FromConfig(*ec.Kubernetes)
	}

	return nil, fmt.Errorf("no executor specified")
}

func PrepareJob(platform platforms.Platform, job schema.Job) (schema.Job, error) {
	tediumImage := job.Config.Images.Tedium

	// add our own steps to the chore (editing in place)

	jobEnvBundle, err := job.ToEnvironment()
	if err != nil {
		return schema.Job{}, fmt.Errorf("error generating job environment variable: %w", err)
	}

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

	// convert chore steps into execution steps

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
	env["TEDIUM_REPO_CLONE_URL"] = job.Repo.CloneUrl
	env["TEDIUM_REPO_DEFAULT_BRANCH"] = job.Repo.DefaultBranch
	env["TEDIUM_PLATFORM_TYPE"] = platform.Config().Type
	env["TEDIUM_PLATFORM_DOMAIN"] = platform.Config().Domain
	env["TEDIUM_PLATFORM_API_BASE_URL"] = platform.ApiBaseUrl()
	env["TEDIUM_PLATFORM_EMAIL"] = platform.Profile().Email
	if job.Chore.SourceConfig.ExposePlatformToken {
		env["TEDIUM_PLATFORM_TOKEN"] = platform.AuthToken()
	}

	for k, v := range step.Environment {
		if !step.Internal && strings.HasPrefix(k, "TEDIUM_") {
			l.Warn("Not passing environment variable to chore step", "key", k)
		} else {
			env[k] = v
		}
	}

	for k, v := range job.Chore.SourceConfig.Environment {
		if strings.HasPrefix(k, "TEDIUM_") {
			l.Warn("Not passing environment variable to chore step", "key", k)
		} else {
			env[k] = v
		}
	}

	return env
}
