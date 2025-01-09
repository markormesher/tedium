package podman

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/containers/podman/v5/libpod/define"
	"github.com/containers/podman/v5/pkg/bindings/containers"
	"github.com/containers/podman/v5/pkg/bindings/images"
	"github.com/markormesher/tedium/internal/utils"
)

var logPrinterLock sync.Mutex

func (p *PodmanExecutor) pullImage(name string) error {
	exists, err := images.Exists(p.conn, name, nil)
	if err != nil {
		return fmt.Errorf("error checking whether image exists before pulling: %w", err)
	}

	if exists {
		l.Debug("Image already exists - not pulling", "image", name)
		return nil
	}

	l.Info("Pulling container image", "image", name)
	_, err = images.Pull(p.conn, name, &images.PullOptions{Quiet: utils.BoolPtr(true)})
	if err != nil {
		return fmt.Errorf("error pulling image: %w", err)
	}

	return nil
}

func (p *PodmanExecutor) waitForContainerCompletion(name string) (int, error) {
	exitCode, err := containers.Wait(p.conn, name, &containers.WaitOptions{
		Condition: []define.ContainerStatus{define.ContainerStateStopped, define.ContainerStateExited},
	})
	if err != nil {
		return -1, fmt.Errorf("error waiting for container to stop: %w", err)
	}

	return int(exitCode), nil
}

func (p *PodmanExecutor) printContainerLogs(name string) error {
	// wait for logs to finish - there can be a slight lag
	time.Sleep(2 * time.Second)

	// acquire a lock for output printing, so we don't mingle logs from multiple containers
	logPrinterLock.Lock()
	defer logPrinterLock.Unlock()

	logPrinter := make(chan string)
	go func() {
		for str := range logPrinter {
			str = strings.TrimSpace(str)
			if str != "" {
				fmt.Println(str)
			}
		}
	}()

	logOpts := containers.LogOptions{
		Follow: utils.BoolPtr(false),
		Stderr: utils.BoolPtr(true),
		Stdout: utils.BoolPtr(true),
	}

	l.Info("START of logs for container", "container", name)
	containers.Logs(p.conn, name, &logOpts, logPrinter, logPrinter)
	close(logPrinter)
	l.Info("END of logs for container", "container", name)

	return nil
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
