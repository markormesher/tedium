package schema

// NOTE: this file is referenced in the README - update any links if you move or rename this file.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ExecutorConfig defines the executor used to perform chores.
type ExecutorConfig struct {
	// ChoreConcurrency defines how many chores Tedium should attempt to run concurrently. It is an upper bound and may not be reached. Defaults to 1.
	ChoreConcurrency int `json:"concurrency" yaml:"concurrency"`

	// Kubernetes defines how to connect to the Kubernetes cluster for chore execution.
	Kubernetes KubernetesConfig `json:"kubernetes" yaml:"kubernetes"`
}

type KubernetesConfig struct {
	// KubeconfigPath locates the configuration used to communicate with Kubernetes. If not supplied, the executable will assume it is running inside Kubernetes and will attempt to use the in-cluster config.
	KubeconfigPath string `json:"kubeconfigPath" yaml:"kubeconfigPath"`

	// Namespace defines where chores are executed. It defaults to "default".
	Namespace string `json:"namespace" yaml:"namespace"`
}

// ExecutionStep decouples the definition of a ChoreStep from the actual execution.
type ExecutionStep struct {
	Image   string `json:"image" yaml:"image"`
	Command string `json:"command" yaml:"command"`

	Label       string
	Environment map[string]string
}

// Job represents an item of work to be done: a specific chore on a specific repo. It should be self-contained; i.e. carry all the info needed to perform a job.
type Job struct {
	Config          TediumConfig
	Repo            Repo
	Chore           ChoreSpec
	ExecutionSteps  []ExecutionStep
	PlatformConfig  PlatformConfig
	WorkBranchName  string
	FinalBranchName string
}

// ToEnvironment bundles the Job into a single environment variable that can be unpacked later by the init and finalise stages of an execution.
func (job *Job) ToEnvironment() (map[string]string, error) {
	jobStrBytes, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("error marshalling Tedium config into environment variable: %w", err)
	}

	return map[string]string{
		"TEDIUM_JOB": string(jobStrBytes),
	}, nil
}

func JobFromEnvironment() (Job, error) {
	jobStr := os.Getenv("TEDIUM_JOB")

	var job Job
	decoder := json.NewDecoder(strings.NewReader(jobStr))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&job)
	if err != nil {
		return Job{}, fmt.Errorf("error decoding job: %w", err)
	}

	return job, nil
}
