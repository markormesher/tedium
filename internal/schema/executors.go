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
	Label       string
	Image       string `json:"image" yaml:"image"`
	Command     string `json:"command" yaml:"command"`
	Environment map[string]string
}

// Job represents an item of work to be done: a specific chore on a specific repo. It should be self-contained; i.e. carry all the info needed to perform a job.
type Job struct {
	Config           *TediumConfig
	Repo             *Repo
	RepoConfig       *ResolvedRepoConfig
	Chore            *ChoreSpec
	ExecutionSteps   []ExecutionStep
	PlatformEndpoint string
}

func (job *Job) ToEnvironment() (map[string]string, error) {
	tediumConfigStr, err := json.Marshal(job.Config)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling Tedium config into environment variable: %w", err)
	}

	choreSpecStr, err := json.Marshal(job.Chore)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling chore spec into environment variable: %w", err)
	}

	repo := job.Repo
	repo.PathOnDisk = "/tedium/repo"
	repoStr, err := json.Marshal(repo)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling repo into environment variable: %w", err)
	}

	repoConfigStr, err := json.Marshal(job.RepoConfig)
	if err != nil {
		return nil, fmt.Errorf("Error marshalling repo config into environment variable: %w", err)
	}

	env := make(map[string]string)
	env["TEDIUM_CONFIG"] = string(tediumConfigStr)
	env["TEDIUM_CHORE_SPEC"] = string(choreSpecStr)
	env["TEDIUM_REPO"] = string(repoStr)
	env["TEDIUM_REPO_CONFIG"] = string(repoConfigStr)
	env["TEDIUM_PLATFORM_ENDPOINT"] = job.PlatformEndpoint

	return env, nil
}

func JobFromEnvironment() (*Job, error) {
	// whole config blobs
	tediumConfigStr := os.Getenv("TEDIUM_CONFIG")
	choreSpecStr := os.Getenv("TEDIUM_CHORE_SPEC")
	repoStr := os.Getenv("TEDIUM_REPO")
	repoConfigStr := os.Getenv("TEDIUM_REPO_CONFIG")

	if tediumConfigStr == "" {
		return nil, fmt.Errorf("Tedium config not present in environment")
	}

	if choreSpecStr == "" {
		return nil, fmt.Errorf("Chore spec not present in environment")
	}

	if repoStr == "" {
		return nil, fmt.Errorf("Repo not present in environment")
	}

	if repoConfigStr == "" {
		return nil, fmt.Errorf("Repo config not present in environment")
	}

	// decode blobs
	var tediumConfig TediumConfig
	decoder := json.NewDecoder(strings.NewReader(tediumConfigStr))
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&tediumConfig)
	if err != nil {
		return nil, fmt.Errorf("Error decoding Tedium config: %w", err)
	}

	var choreSpec ChoreSpec
	decoder = json.NewDecoder(strings.NewReader(choreSpecStr))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&choreSpec)
	if err != nil {
		return nil, fmt.Errorf("Error decoding chore spec: %w", err)
	}

	var repo Repo
	decoder = json.NewDecoder(strings.NewReader(repoStr))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&repo)
	if err != nil {
		return nil, fmt.Errorf("Error decoding repo: %w", err)
	}

	var repoConfig ResolvedRepoConfig
	decoder = json.NewDecoder(strings.NewReader(repoConfigStr))
	decoder.DisallowUnknownFields()
	err = decoder.Decode(&repoConfig)
	if err != nil {
		return nil, fmt.Errorf("Error decoding repo config: %w", err)
	}

	// simple values
	platformEndpoint := os.Getenv("TEDIUM_PLATFORM_ENDPOINT")

	if platformEndpoint == "" {
		return nil, fmt.Errorf("Platform endpoint not present in environment")
	}

	return &Job{
		Config:           &tediumConfig,
		Repo:             &repo,
		RepoConfig:       &repoConfig,
		Chore:            &choreSpec,
		PlatformEndpoint: platformEndpoint,
	}, nil
}
