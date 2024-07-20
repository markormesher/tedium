![CircleCI](https://img.shields.io/circleci/build/github/markormesher/tedium)

# Tedium

Tedium is a tool to automate the execution of boring or repetitive tasks, called "chores", across all of your git repos. All chores run in containers, providing complete control over the tooling available. If running a chore against a repo results in changes, Tedium will push those changes and open or update a PR for you.

## Usage

The default way to run Tedium is via its container image. The default container command expects a [config file](#configuration) at `/tedium/config.json` but this can be overridden by specifying your own command.

You can run the container locally, in an orchestration tool like Kubernetes, or in any other way you prefet. For example, to run Tedium locally on your machine, use the following (replace `podman` with `docker` if required):

`podman run -it --rm -v ./config.json:/tedium/config.json ghcr.io/markormesher/tedium:latest`

Alternatively, you can run Tedium from the executable:

`./tedium --config config.json`

Or from directly from source:

`go run -tags remote ./cmd/tedium.go --config ./config.json`

## Concepts

There are a few key concepts within Tedium: chores, executors, and platforms.

### Chores

Chores are the boring, repeatable tasks that Tedium runs for you. Each chore contains one or more steps, each of which is defined as a container image reference and a command or script to run ininside that container. Tedium handles mounting the repo to each container and persisting changes between steps.

For example, this very basic chore will tidy up the `go.mod` file anywhere it exists:

```yaml
name: "Tidy go.mod",
description: "Run `go mod tidy` if a go.mod file exists",
steps:
  - image: "docker.io/golang:1.22.5"
    command: |
      cd /tedium/repo
      if [ -f go.mod ]; then go mod tidy; fi
```

_Note: ideally you should enforce that `go.mod` is tidy as part of pre-merge CI checks; this is just an example._

By default, Tedium adds extra steps at the beginning and end of each chore:

- Pre-chore: before running chore steps, Tedium will clone the repo and check out a branch for the chore, reusing an existing one if it already exists.
- Post-chore: after the chore steps finish, Tedium will commit any changes, push them to the repo's platform, and open or update a PR.

These pre-chore and post-chore steps can be disabled if required (for example if your chore never makes changes, but does something like call an API to enforce repository settings).

### Executors

Tedium itself doesn't actually execute the steps within chores: it uses container orchestrators as executors to do the work.

The primary executor is Kubernetes, as Tedium is designed to run on a regular cadence with something like a [Kubernetes CronJob](https://kubernetes.io/docs/concepts/workloads/controllers/cron-jobs), but Podman is also supported for local execution.

### Platforms

Platforms are where repos are hosted. Tedium uses them to discover repos to operate on, pull and push them, and manage PRs.

So far only Gitea is supported, but [GitHub support](https://github.com/markormesher/tedium/issues/1) is coming soon.

Each run of Tedium can target multiple platforms at the same time (see [Configuration](#configuration) below).

## Configuration

TODO
