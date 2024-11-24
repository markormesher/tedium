package executors

import (
	"fmt"
	"strings"

	"github.com/markormesher/tedium/internal/executors/kubernetes"
	"github.com/markormesher/tedium/internal/executors/podman"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
)

var l = logging.Logger

func FromExecutorConfig(ec *schema.ExecutorConfig) (schema.Executor, error) {
	switch {
	case ec.Podman != nil:
		return podman.FromConfig(ec.Podman)

	case ec.Kubernetes != nil:
		return kubernetes.FromConfig(ec.Kubernetes)
	}

	return nil, fmt.Errorf("No executor specified")
}

func PrepareJob(job *schema.Job) error {
	tediumImage := job.Config.Images.Tedium

	// add our own steps to the chore (editing in place)

	if !job.Chore.SkipCloneStep {
		tediumStep := schema.ChoreStep{
			Image:   tediumImage,
			Command: "/app/tedium --internal-command initChore",
		}
		job.Chore.Steps = append([]schema.ChoreStep{tediumStep}, job.Chore.Steps...)
	}

	if !job.Chore.SkipFinaliseStep {
		tediumStep := schema.ChoreStep{
			Image:   tediumImage,
			Command: "/app/tedium --internal-command finaliseChore",
		}
		job.Chore.Steps = append(job.Chore.Steps, tediumStep)
	}

	// convert chore steps into execution steps

	baseEnv, err := job.ToEnvironment()
	if err != nil {
		return fmt.Errorf("Error generating environment variables for job: %w", err)
	}

	job.ExecutionSteps = make([]schema.ExecutionStep, len(job.Chore.Steps))
	for i := range job.Chore.Steps {
		step := job.Chore.Steps[i]
		job.ExecutionSteps[i] = schema.ExecutionStep{
			Label:       fmt.Sprintf("step-%d", i+1),
			Image:       step.Image,
			Command:     step.Command,
			Environment: envForStep(baseEnv, job.Chore, &step),
		}
	}

	return nil
}

func envForStep(baseEnv map[string]string, chore *schema.ChoreSpec, step *schema.ChoreStep) map[string]string {
	env := make(map[string]string)
	for k, v := range baseEnv {
		env[k] = v
	}

	env["TEDIUM_COMMAND"] = step.Command

	for k, v := range step.Environment {
		if strings.HasPrefix(k, "TEDIUM_") {
			l.Warn("Not passing environment variable to chore step", "key", k)
		} else {
			env[k] = v
		}
	}

	for k, v := range chore.UserProvidedEnvironment {
		if strings.HasPrefix(k, "TEDIUM_") {
			l.Warn("Not passing environment variable to chore step", "key", k)
		} else {
			env[k] = v
		}
	}

	return env
}
