package podman

import (
	"context"
	"fmt"
	"os/user"
	"time"

	"github.com/containers/podman/v5/pkg/bindings"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
)

var l = logging.Logger

type PodmanExecutor struct {
	SocketPath string

	// private state
	conf *schema.TediumConfig
	conn context.Context

	volumeNames    []string
	containerNames []string
}

func FromConfig(c *schema.PodmanExecutorConfig) (*PodmanExecutor, error) {
	return &PodmanExecutor{
		SocketPath: c.SocketPath,
	}, nil
}

func (p *PodmanExecutor) Init(conf *schema.TediumConfig) error {
	p.conf = conf

	if p.SocketPath == "" {
		l.Info("No Podman socket provided - will attempt to use a default value")
		user, err := user.Current()
		if err != nil {
			p.SocketPath = "unix:///run/podman/podman.sock"
			l.Info("Error determining current user - using the root-owned socket", "socketPath", p.SocketPath)
		} else if user.Uid == "0" {
			p.SocketPath = "unix:///run/podman/podman.sock"
			l.Info("User is root - using the root-owned socket", "socketPath", p.SocketPath)
		} else {
			p.SocketPath = fmt.Sprintf("unix:///run/user/%s/podman/podman.sock", user.Uid)
			l.Info("Using the user-owned socket", "socketPath", p.SocketPath)
		}
	}

	conn, err := bindings.NewConnection(context.Background(), p.SocketPath)
	if err != nil {
		return fmt.Errorf("Error creating Podman binding: %w", err)
	}
	p.conn = conn

	// keep track of resources so we can be sure to clean up later
	p.volumeNames = make([]string, 0)
	p.containerNames = make([]string, 0)

	return nil
}

func (p *PodmanExecutor) Deinit() error {
	time.Sleep(5 * time.Second)
	p.cleanUpContainers()
	p.cleanUpVolumes()

	return nil
}

func (p *PodmanExecutor) ExecuteChore(job *schema.Job) error {
	repoVolume, err := p.createVolume("repo")
	if err != nil {
		return err
	}

	for i := range job.ExecutionSteps {
		step := job.ExecutionSteps[i]

		s := specgen.NewSpecGenerator(step.Image, false)
		s.Name = utils.UniqueName(step.Label)
		s.Command = []string{"/bin/sh", "-c", "echo \"${TEDIUM_COMMAND}\" | /bin/sh"}
		s.Env = step.Environment
		s.Volumes = []*specgen.NamedVolume{
			{
				Name: repoVolume,
				Dest: "/tedium/repo",
			},
		}

		err = p.runContainerToCompletion(s)
		if err != nil {
			return err
		}
	}

	return nil
}
