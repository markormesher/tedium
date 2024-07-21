![CircleCI](https://img.shields.io/circleci/build/github/markormesher/tedium)

# Tedium

Tedium is a tool to automate the execution of boring or repetitive tasks, called "chores", across all of your Git repos. All chores run in containers, providing complete control over the tooling available. If running a chore against a repo results in changes, Tedium will push those changes and open or update a PR for you.

## 💻 Usage

The default way to run Tedium is via its container image. The default container command expects a [config file](#-configuration) at `/tedium/config.json` but this can be overridden by specifying your own command.

You can run the container locally, in an orchestration tool like Kubernetes, or in any other way you prefet. For example, to run Tedium locally on your machine, use the following (replace `podman` with `docker` if required):

`podman run -it --rm -v ./config.json:/tedium/config.json ghcr.io/markormesher/tedium:latest`

Alternatively, you can run Tedium from the executable:

`./tedium --config config.json`

Or from directly from source:

`go run -tags remote ./cmd/tedium.go --config ./config.json`

## 📖 Concepts

There are a few key concepts within Tedium: chores, executors, and platforms.

### Chores

Chores are the boring, repeatable tasks that Tedium runs for you. Tedium will execute chores against your repos, and if they cause any changes to the repo it will commit them to a branch and open or update a PR.

Running chores is the entire point of Tedium, so see [Chores](#-chores) below for lots more detail.

### Executors

Tedium itself doesn't actually execute the steps within chores: it uses container orchestrators as executors to do the work.

The primary executor is Kubernetes, as Tedium is designed to run on a regular cadence with something like a [Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs), but Podman is also supported for local execution.

### Platforms

Platforms are where repos are hosted. Tedium uses them to discover repos to operate on, pull and push them, and manage PRs.

So far only Gitea is supported, but [GitHub support](https://github.com/markormesher/tedium/issues/1) is coming soon.

Each run of Tedium can target multiple platforms at the same time (see [Configuration](#-configuration) below).

## ✨ Features

### Auto-Enrollment

TODO

### Repo Config Inheritance

Repo configuration can extend one or more other configurations, allowing a common list of chores to easily be applied across many repos. Each inherited configuration is defined as a simple repo URL; the config is expected to live at `index.{json,yml,yaml}` inside that repo.

The list of chores to apply is merged from all extended configurations. Extension is recursive, but recursion will safely abort if a loop is detected.

## 🔧 Configuration

Tedium is configured in two place:

- **Runtime configuration:** the configuration file passed to the Tedium executable when it runs.
- **Repo configuration:** a configuration file inside each repo.

Additionally, chores need to be defined in order for Tedium to do anything useful - see [Chores](#-chores) below.

### Runtime Configuration

Runtime configuration is provided to Tedium on the command line when it is executed (see [Usage](#-usage)) to control how the program should run. It can be provided as JSON or YAML.

The full schema of runtime configuration is defined in [schema/config.go](./internal/schema/config.go) as `TediumConfig`. An example is provided below, but **do not copy this as-is** - you will need to change it before it can be used.

```yaml
# The executor used to execute chores - you must supply ONE value.
# Required.
executor:

  # If you're running chores locally with Podman:
  podman:
    # Optional, several defaults will be tried if not supplied.
    socketPath: "unix:///run/podman/podman.sock"

  # If you're running chores in a Kubernetes cluster:
  kubernetes:
    # Required when running the executor locally, optional when running it inside the cluster.
    kubeConfigPath: "~/.kube/config"

    # Namespace to execute chores in.
    # The namespace must exist; Tedium will not create it if it doesn't.
    # Optional, defaults to "default".
    namespace: "tedium"

# Platforms to discover repos from.
# Required.
platforms:

    # Platform type; "gitea" only for now, "github" support coming soon.
    # Required.
  - type: "gitea"

    # Required.
    endpoint: "https://gitea.example.com/api/v1"

    # List of regexes to filter repos against during discovery.
    # Repos matching any filter will be included.
    # The string tested is "org-name/repo-name". Regex format is as-per the Go standard library.
    # Optional, defaults to all repos being included.
    repoFilters:
      - "myorg/example\\-.*"

    # Auth for this platform.
    # Values follow the same format as auth config below.
    # Takes priority over per-domain auth defined below.
    # Optional.
    auth:
      token: "abc123"

# Per-domain configuration of platform authentication. Keys must be domains.
# Used when discovering repos and any time repos are cloned (chores, extended configs, etc).
# Optional.
auth:

  gitea.example.com:
    # Token auth is the only supported mechanism at the moment.
    token: "abc123"

# Location on disk to store repos that are cloned (target repos, chores, extended configs, etc).
# Optional, defaults to a temporary path.
repoStoragePath: "/tmp/tedium"

# Container images used for in-built stages of chores.
# Optional.
images:

  # Tedium image used for pre- and post-chore steps.
  # Optional, defaults to latest.
  tedium: "ghcr.io/markormesher/tedium:latest"

  # Placeholder image used by the Kubernetes executor.
  # Optional, defaults to latest.
  pause: "ghcr.io/markormesher/tedium-pause:latest"

# Auto-enrollment settings for discovered repos that don't already have a repo configuration file.
# Optional, defaults to disabled.
autoEnrollment:

  # Optional, defaults to false.
  enabled: true

  # Repo config to add to any repo (via PR) that is auto-enrolled.
  # Value must be a valid repo configuration (see below).
  # Required if auto-enrollment is enabled, optional otherwise.
  config:
    extends:
      - "https://github.com/example/tedium-config-all-repos.git"
      - "https://github.com/example/tedium-config-go-projects.git"
    chores: []
```

### Repo Configuration

Repo configuration is committed to the repo and defines how Tedium should handle that repo after it has been discovered from a platform. It must be in the root directory of the repo and named `.tedium.{json,yml,yaml}`.

If no repo configuration file exists then Tedium will skip that repo, unless [auto-enrollment](#auto-enrollment) is enabled.

The full schema of repo configuration is defined in [schema/config.go](./internal/schema/config.go) as `RepoConfig`. An example is provided below, but **do not copy this as-is** - you will need to change it before it can be used.

```yaml
# URLs of repos containing more repo config to apply to this repo.
# Optional.
extends:
  - "https://github.com/example/tedium-config-all-repos.git"
  - "https://github.com/example/tedium-config-go-projects.git"

# Chores to execute against this repo.
# Each chore is defined as a Git repo URL and a directory within that repo.
# Optional.
chores:
  - cloneUrl: "https://github.com/example/my-tedium-chores.git",
    directory: "tidy-go-mod"
```

## 🧹 Chores

Chores are very simple: they are a series of steps to run against a repo, each of which is defined as a container image reference and a command to run ininside that container.

Tedium handles mounting the repo contents at `/tedium/repo` in each container and persisting changes between steps.

By default, Tedium adds extra steps at the beginning and end of each chore:

- **Pre-chore:** before running chore steps, Tedium will clone the repo and check out a branch for the chore, reusing an existing one if it already exists.
- **Post-chore:** after the chore steps finish, Tedium will commit any changes, push them to the repo's platform, and open or update a PR.

These pre-chore and post-chore steps can be disabled if required (for example if your chore never makes changes, but does something like call an API to enforce repository settings).

### Definition

Chores live in dedicated repos, organised into directories, as shown above.

The full schema of a chore is defined in [schema/chores.go](./internal/schema/chores.go) as `ChoreSpec`. An example is provided below, but **do not copy this as-is** - you will need to change it before it can be used.

```yaml
# Name of this chore. This will be visible to users in PR titles.
# Required.
name: "Tidy go.mod",

# Description of this chore. This will be visible to users in PR bodies.
# Optional but highly recommended.
description: "Run `go mod tidy` if a go.mod file exists",

# Steps for this chore.
# Required.
steps:

    # Image to use for this step.
    # Required.
  - image: "docker.io/golang:1.22.5"

    # Command to run in this step. This will be piped to `/bin/sh` in the container.
    # Required.
    command: |
      cd /tedium/repo
      if [ -f go.mod ]; then go mod tidy; fi

    # Environment variables to inject into the container.
    # Tedium will also provide some utility values, defined in the ToEnvironment() method in internal/schema/executors.go.
    # Optional.
    environment:
      MY_VAR_1: "foo"
      MY_VAR_2: "bar"

# If true, skip the pre-chore step to clone the repo.
# Optional, defaults to false.
skipCloneStep: false

# If true, skip the post-chore step to commit and push any changes.
# Optional, defaults to false.
skipFinaliseStep: false
```
