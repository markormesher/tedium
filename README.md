![CircleCI](https://img.shields.io/circleci/build/github/markormesher/tedium)

# Tedium

Tedium is a tool to automate the execution of boring or repetitive tasks, called "chores", across all of your Git repos. All chores run in containers, providing complete control over the tooling available. If running a chore against a repo results in changes, Tedium will push those changes on a branch and open or update a PR for you.

## 💻 Usage

The best way to run Tedium is via its container image. The default container command expects a [config file](#-configuration) at `/tedium/config.yml` but this can be overridden by specifying your own command.

You can run the container locally, in an orchestration tool like Kubernetes, or in any other way you prefer. For example, to run Tedium locally on your machine, use the following:

```shell
# replace `podman` with `docker` if required
podman run -it --rm -v ./config.yml:/tedium/config.yml ghcr.io/markormesher/tedium:latest
```

Alternatively, you can run Tedium from the executable:

```shell
./tedium --config config.yml
```

Or from directly from source:

```shell
go run -tags remote ./cmd/tedium.go --config ./config.yml
```

## 📖 Concepts

There are a few key concepts within Tedium: chores, executors, and platforms.

### Chores

Chores are the boring, repeatable tasks that Tedium runs for you. Tedium will execute chores against your repos, and if they cause any changes to the repo it will commit them to a branch and open or update a PR.

Running chores is the entire point of Tedium, so see [Chores](#-chores) below for lots more detail.

### Executors

Executors are the container orchestrators that Tedium uses to clone your repos, actually run your chores, and push any changes.

The primary executor is Kubernetes, as Tedium is designed to run on a regular cadence with something like a [Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs), but Podman is also supported for local execution.

### Platforms

Platforms are where repos are hosted. Tedium uses them to discover repos to operate on, pull and push them, manage PRs, and read config files.

GitHub and Gitea are supported, and each run of Tedium can target multiple platforms at the same time (see [Configuration](#-configuration) below).

**Important note:** Tedium will never talk to a platform you haven't told it to, even just to read a config file. If you want to be able to read config files from a platform without operating on the repos there (e.g. to read config from public GitHub but only execute chores against repos in your private Gitea instance), configure the platform with `skipDiscovery: true`.

## ✨ Features

### Auto-Enrollment

If Tedium encounters a repo that lacks a [repo config](#repo-config) file, it can optionally open a PR to auto-enroll that repo with a config that you define. This makes it easier to adopt using Tedium across your repos and helps to stop new repos from being left behind.

See `.autoEnrollment` under [runtime configuration](#runtime-configuration).

### Repo Config Inheritance

Repo configuration can extend one or more other configurations, allowing a common list of chores to easily be applied across many repos. Each inherited configuration is defined as a simple repo URL; the config is expected to live at `index.{yml,yaml,json}` inside that repo.

The list of chores to apply is merged from all extended configurations. Extension is recursive and will safely abort if a loop is detected.

See `.extends` under [repo configuration](#repo-configuration).

## 🔧 Configuration

Tedium is configured in two place:

- **Runtime configuration:** the configuration file passed to the Tedium executable when it runs.
- **Repo configuration:** a configuration file inside each repo.

### Runtime Configuration

Runtime configuration is provided to Tedium on the command line when it is executed (see [Usage](#-usage)) to control how the program should run. It can be provided as YAML or JSON.

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

    # Platform type ("gitea" or "github")
    # Required.
  - type: "gitea"

    # Platform domain. Don't include any path segments. For GitHub instances, do not include the "api." subdomain.
    # Required.
    domain: "gitea.example.com"

    # List of regexes to filter repos against during discovery.
    # Repos matching any filter will be included.
    # The string tested is "org-name/repo-name". Regex format is as-per the Go standard library.
    # Optional, defaults to all repos being included.
    repoFilters:
      - "myorg/example\\-.*"

    # Skip discovery of repos from this platform, only use it to read config files.
    # Optional, defaults to false.
    skipDiscovery: false

    # Auth for this platform.
    # Values follow the same format as auth config below.
    # Required, unless this platform will only be used to read public config files.
    auth:
      type: "user_token"
      token: "abc123"

# Container images used for built-in chore steps.
# Optional.
images:

  # Tedium image used for pre- and post-chore steps.
  # Optional, defaults to the latest tag within the same major version.
  tedium: "ghcr.io/markormesher/tedium:v0"

  # Placeholder image used by the Kubernetes executor.
  # Optional, defaults to the latest tag within the same major version.
  pause: "ghcr.io/markormesher/tedium-pause:v0"

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
      - "https://github.com/example/tedium-config-all-repos"
      - "https://github.com/example/tedium-config-go-projects"
    chores: []

# How many chores to attempt to run at once (upper bound - actual concurrency may be lower).
# Optional, defaults to 1.
choreConcurrency: 5
```

<details>
<summary>Auth Configuration</summary>

Tedium can act as a user or an application when interacting with Git platforms, depending on how authentication is configured.

#### Acting as a User

- Set `type: "user_token"`.
- Generate a token for your Tedium service user and provide it in the `token` field.
  - The token needs read/write permissions on contents, issues, and pull requests. For GitHub it must be a "classic" token, as new-style fine-grain tokens do not yet allow you to push to repos as a collaborator.
  - Note that the user must be a collaborator on your repositories.
  - It is *not* recommended to use a token for your own personal user.

#### Acting as an Application

- Set `type: "app"`.
- To create an application:
  - On GitHub: Settings > Developer Settings > New GitHub App
    - The app needs read/write permissions on contents, issues, and pull requests.
    - After installing the app the installation ID can be found can be found at the end of the URL on the app settings page.
  - On Gitea: TODO
- Provide the `clientId` and `privateKey` or `privateKeyFile` for your app, and the `installationId` for its installation in your profile/organisation.
</details>

### Repo Configuration

Repo configuration is committed to a repo and defines how Tedium should handle that repo after it has been discovered from a platform. It must be in the root directory of the repo and named `.tedium.{yml,yaml,json}`.

If no repo configuration file exists then Tedium will skip that repo, unless [auto-enrollment](#auto-enrollment) is enabled.

The full schema of repo configuration is defined in [schema/config.go](./internal/schema/config.go) as `RepoConfig`. An example is provided below, but **do not copy this as-is** - you will need to change it before it can be used.

```yaml
# URLs of repos containing more repo config to apply to this repo.
# These links will be followed recursively and all discovered configs will be merged. Chores will be dedupicated, based on their URL and directory.
# Optional.
extends:
  - "https://github.com/example/tedium-config-all-repos"
  - "https://github.com/example/tedium-config-go-projects"

# Chores to execute against this repo.
# Each chore is defined as a Git repo URL and a directory within that repo, plus extra optional configuration as below.
# If a chore has the same URL and domain as one discovered from the `extends` config above their definitions will be merged.
# Optional.
chores:
  - url: "https://github.com/example/my-tedium-chores",
    directory: "render-circle-ci"

    # The branch to read this chore from, if not the default.
    # Optional.
    branch: "my-experimental-change"

    # Expose the platform's auth token to chore steps via the TEDIUM_PLATFORM_TOKEN environment variable. Use with caution.
    # Optional, defaults to false.
    exposePlatformToken: true

    # Additional environment variables to pass to every step of the chore. Must not start with "TEDIUM_".
    # Optional.
    environment:
      FOO: "bar"
```

## 🧹 Chores

Chores are pretty simple: they are a series of steps to run against a repo, each of which is defined as a container image and a command to run ininside that container.

Tedium handles mounting the repo contents at `/tedium/repo` in each container and persisting changes between steps.

By default, Tedium adds extra steps at the beginning and end of each chore:

- **Pre-chore:** before running chore steps, Tedium will clone the repo and check out a branch for the chore, reusing an existing one if it already exists.
- **Post-chore:** after the chore steps finish, Tedium will commit any changes, push them to the repo's platform, and open or update a PR.

These pre-chore and post-chore steps can be disabled if required (for example if your chore never makes changes, but does something like call an API to enforce repository settings).

### Definition

Chores live in dedicated repos, organised into directories, as shown above. The definition file is expected to live at `chore.{yml,yaml,json}` within the directory; no other files from the directory will be read by Tedium, but it's not a problem if other file are present (for example, you might use that directory for the `Containerfile` and any scripts needed to build your chore image).

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
    # Tedium will also provide some values, defined in the ToEnvironment() method in internal/schema/executors.go.
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
