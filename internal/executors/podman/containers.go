package podman

import (
	"fmt"
	"strings"
	"time"

	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/markormesher/tedium/internal/utils"
)

func printLogs(ch <-chan string) {
	for {
		str := <-ch
		str = strings.TrimSpace(str)
		if len(str) > 0 {
			fmt.Println(str)
		}
	}
}

func (p *PodmanExecutor) runContainerToCompletion(spec *specgen.SpecGenerator) error {
	p.containerNames = append(p.containerNames, spec.Name)

	p.pullImage(spec.Image)

	createResponse, err := containers.CreateWithSpec(p.conn, spec, nil)
	if err != nil {
		return fmt.Errorf("Error creating container from spec: %w", err)
	}

	stdOutPrinter := make(chan string)
	stdErrPrinter := make(chan string)
	go printLogs(stdOutPrinter)
	go printLogs(stdErrPrinter)

	defer func() {
		close(stdOutPrinter)
		close(stdErrPrinter)
	}()

	go containers.Logs(p.conn, createResponse.ID, &containers.LogOptions{Follow: utils.BoolPtr(true)}, stdOutPrinter, stdErrPrinter)

	l.Info("Starting container", "container", spec.Name)
	err = containers.Start(p.conn, createResponse.ID, nil)
	if err != nil {
		return fmt.Errorf("Error starting container: %w", err)
	}

	// TODO: handle reaching other statuses
	exitCode, err := containers.Wait(p.conn, spec.Name, &containers.WaitOptions{
		Condition: []define.ContainerStatus{define.ContainerStateStopped},
	})
	if err != nil {
		return fmt.Errorf("Error waiting for container to stop: %w", err)
	}

	// TODO: find a better way of waiting for logs to finish
	time.Sleep(2 * time.Second)

	if exitCode != 0 {
		return fmt.Errorf("Container finished with a non-zero exit code: %d", exitCode)
	}

	return nil
}

func (p *PodmanExecutor) pullImage(name string) error {
	exists, err := images.Exists(p.conn, name, nil)
	if err != nil {
		return fmt.Errorf("Error checking whether image exists before pulling: %w", err)
	}

	if exists {
		l.Debug("Image already exists - not pulling", "image", name)
		return nil
	}

	l.Info("Pulling container image", "image", name)
	_, err = images.Pull(p.conn, name, nil)
	if err != nil {
		return fmt.Errorf("Error pulling image: %w", err)
	}

	return nil
}

func (p *PodmanExecutor) cleanUpContainers() {
	for _, containerName := range p.containerNames {
		p.deleteContainerIfExists(containerName)
	}
}

func (p *PodmanExecutor) deleteContainerIfExists(name string) {
	exists, err := containers.Exists(p.conn, name, nil)
	if err != nil {
		l.Error("Error checking if container exists before deleting", "error", err)
	}

	if !exists {
		return
	}

	container, err := containers.Inspect(p.conn, name, nil)
	if err != nil {
		l.Error("Error inspecting container before deleting", "error", err)
	}

	if container.State.Running {
		l.Warn("Cleaning up a container that is still running - this is bad!", "name", container.Name)
	}

	_, err = containers.Remove(p.conn, name, &containers.RemoveOptions{
		Force: utils.BoolPtr(true),
	})
	if err != nil {
		l.Error("Error deleting container", "error", err)
	}
}
