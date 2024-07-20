package podman

import (
	"fmt"

	"github.com/containers/podman/v5/pkg/bindings/volumes"
	podmanTypes "github.com/containers/podman/v5/pkg/domain/entities/types"
	"github.com/markormesher/tedium/internal/utils"
)

func (p *PodmanExecutor) createVolume(role string) (string, error) {
	name := utils.UniqueName(role)

	_, err := volumes.Create(p.conn, podmanTypes.VolumeCreateOptions{
		Name: name,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("Error creating volume '%s': %w", name, err)
	}

	p.volumeNames = append(p.volumeNames, name)

	return name, nil
}

func (p *PodmanExecutor) cleanUpVolumes() {
	for _, volumeName := range p.volumeNames {
		p.deleteVolumeIfExists(volumeName)
	}
}

func (p *PodmanExecutor) deleteVolumeIfExists(name string) {
	exists, err := volumes.Exists(p.conn, name, nil)
	if err != nil {
		l.Error("Error checking if volume exists before deleting", "error", err)
	}

	if exists {
		err := volumes.Remove(p.conn, name, nil)
		if err != nil {
			l.Error("Error deleting volume", "error", err)
		}
	}
}
