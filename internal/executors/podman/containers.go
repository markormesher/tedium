package podman

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/containers/podman/v5/pkg/specgen"
	"github.com/markormesher/tedium/internal/utils"
)

var logPrinterLock sync.Mutex

func (p *PodmanExecutor) runContainerToCompletion(spec *specgen.SpecGenerator) error {
	p.containerNames = append(p.containerNames, spec.Name)

	p.pullImage(spec.Image)

	createResponse, err := containers.CreateWithSpec(p.conn, spec, nil)
	if err != nil {
		return fmt.Errorf("Error creating container from spec: %w", err)
	}

	l.Info("Starting container", "container", spec.Name)
	err = containers.Start(p.conn, createResponse.ID, nil)
	if err != nil {
		return fmt.Errorf("Error starting container: %w", err)
	}

	exitCode, err := containers.Wait(p.conn, spec.Name, &containers.WaitOptions{
		Condition: []define.ContainerStatus{define.ContainerStateStopped, define.ContainerStateExited},
	})
	if err != nil {
		return fmt.Errorf("Error waiting for container to stop: %w", err)
	}
	l.Info("Container finished", "container", spec.Name, "exitCode", exitCode)

	// wait for logs to finish - there can be a slight lag
	time.Sleep(2 * time.Second)

	// acquire a lock for output printing, so we don't mingle logs from multiple containers
	logPrinterLock.Lock()
	defer logPrinterLock.Unlock()

	l.Info("START of logs for container", "container", spec.Name)
	logPrinter := make(chan string)
	go func() {
		for str := range logPrinter {
			str = strings.TrimSpace(str)
			if len(str) > 0 {
				fmt.Println(str)
			}
		}
	}()

	logOpts := containers.LogOptions{
		Follow: utils.BoolPtr(false),
		Stderr: utils.BoolPtr(true),
		Stdout: utils.BoolPtr(true),
	}

	containers.Logs(p.conn, createResponse.ID, &logOpts, logPrinter, logPrinter)
	close(logPrinter)

	l.Info("END of logs for container", "container", spec.Name)

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
	_, err = images.Pull(p.conn, name, &images.PullOptions{Quiet: utils.BoolPtr(true)})
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
