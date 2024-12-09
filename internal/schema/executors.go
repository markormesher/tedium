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
	Podman     *PodmanExecutorConfig     `json:"podman" yaml:"podman"`
	Kubernetes *KubernetesExecutorConfig `json:"kubernetes" yaml:"kubernetes"`
}

type PodmanExecutorConfig struct {
	// SocketPath identifies the socket used to communicate with Podman. If not supplied, several default values will be tried.
	SocketPath string `json:"socketPath" yaml:"socketPath"`
}

type KubernetesExecutorConfig struct {
	// KubeconfigPath locates the configuration used to communicate with Kubernetes. If not supplied, the executable will assume it is running inside Kubernetes and will attempt to use the in-cluster config.
	KubeconfigPath string `json:"kubeconfigPath" yaml:"kubeconfigPath"`

	// Namespace defines where chores are executed. It defaults to "default".
	Namespace string `json:"namespace" yaml:"namespace"`
}

type Executor interface {
	Init(conf *TediumConfig) error
	Deinit() error
	ExecuteChore(job *Job) error
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
	Config          *TediumConfig
	Repo            *Repo
	RepoConfig      *ResolvedRepoConfig
	Chore           *ChoreSpec
	ExecutionSteps  []ExecutionStep
	PlatformConfig  *PlatformConfig
	WorkBranchName  string
	FinalBranchName string
}

// ToEnvironment() generates a set of environment variables that are passed into chore execution steps.
func (job *Job) ToEnvironment() (map[string]string, error) {
	env := make(map[string]string, 0)

	// used directly by Tedium for the init and finalise steps
	jobStrBytes, err := json.Marshal(job)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling Tedium config into environment variable: %w", err)
	}
	env["TEDIUM_JOB"] = string(jobStrBytes)

	// made available for convenience in actual chore steps
	env["TEDIUM_REPO_OWNER"] = job.Repo.OwnerName
	env["TEDIUM_REPO_NAME"] = job.Repo.Name
	// ...more

	return env, nil
}

func JobFromEnvironment() (*Job, error) {
	jobStr := os.Getenv("TEDIUM_JOB")

	var job Job
	decoder := json.NewDecoder(strings.NewReader(jobStr))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&job)
	if err != nil {
		return nil, fmt.Errorf("Error decoding job: %w", err)
	}

	return &job, nil
}
