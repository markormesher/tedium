package podman

import (
	"context"
	"fmt"
	"os/user"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

var l = logging.Logger

type PodmanExecutor struct {
	SocketPath string

	// private state
	conf schema.TediumConfig
	conn context.Context
}

func FromConfig(c schema.PodmanExecutorConfig) (*PodmanExecutor, error) {
	return &PodmanExecutor{
		SocketPath: c.SocketPath,
	}, nil
}

func (p *PodmanExecutor) Init(conf schema.TediumConfig) error {
	p.conf = conf

	if p.SocketPath == "" {
		l.Info("No Podman socket provided - will attempt to use a default value")
		usr, err := user.Current()
		switch {
		case err != nil:
			p.SocketPath = "unix:///run/podman/podman.sock"
			l.Info("Error determining current user - using the root-owned socket", "socketPath", p.SocketPath)

		case usr.Uid == "0":
			p.SocketPath = "unix:///run/podman/podman.sock"
			l.Info("User is root - using the root-owned socket", "socketPath", p.SocketPath)

		default:
			p.SocketPath = fmt.Sprintf("unix:///run/user/%s/podman/podman.sock", usr.Uid)
			l.Info("Using the user-owned socket", "socketPath", p.SocketPath)
		}
	}

	conn, err := bindings.NewConnection(context.Background(), p.SocketPath)
	if err != nil {
		return fmt.Errorf("error creating Podman binding: %w", err)
	}
	p.conn = conn

	return nil
}

func (p *PodmanExecutor) ExecuteChore(job schema.Job) error {
	// create a temporary volume to hold the target repo and defer its cleanup
	repoVolume, err := p.createVolume("repo")
	if err != nil {
		return err
	}

	defer func() {
		p.deleteVolumeIfExists(repoVolume)
	}()

	// run each step in a new container
	for _, step := range job.ExecutionSteps {
		err := func() error {
			spec := specgen.NewSpecGenerator(step.Image, false)
			spec.Name = utils.UniqueName(step.Label)
			spec.Command = []string{"/bin/sh", "-c", "echo \"${TEDIUM_COMMAND}\" | /bin/sh"}
			spec.Env = step.Environment
			spec.Volumes = []*specgen.NamedVolume{
				{
					Name: repoVolume,
					Dest: "/tedium/repo",
				},
			}

			p.pullImage(spec.Image)

			createResponse, err := containers.CreateWithSpec(p.conn, spec, nil)
			if err != nil {
				return fmt.Errorf("error creating container from spec: %w", err)
			}

			defer func() {
				p.deleteContainerIfExists(spec.Name)
			}()

			l.Info("Starting container", "container", spec.Name)
			err = containers.Start(p.conn, createResponse.ID, nil)
			if err != nil {
				return fmt.Errorf("error starting container: %w", err)
			}

			exitCode, err := p.waitForContainerCompletion(spec.Name)
			if err != nil {
				return fmt.Errorf("error waiting for container to complete: %w", err)
			}
			l.Info("container finished", "container", spec.Name, "exitCode", exitCode)

			err = p.printContainerLogs(spec.Name)
			if err != nil {
				return fmt.Errorf("error printing container logs: %w", err)
			}

			if exitCode != 0 {
				return fmt.Errorf("container finished with a non-zero exit code: %d", exitCode)
			}

			return nil
		}()

		if err != nil {
			return err
		}
	}

	return nil
}
