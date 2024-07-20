package kubernetes

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func (executor *KubernetesExecutor) getContainerStatus(podName string, containerIdx int) (*v1.ContainerStatus, error) {
	pod, err := executor.podClient.Get(k8sExecutorContext, podName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return &pod.Status.ContainerStatuses[containerIdx], nil
}

func (executor *KubernetesExecutor) waitForContainerCompletion(podName string, containerIdx int) error {
	return wait.PollUntilContextTimeout(k8sExecutorContext, 1*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		status, err := executor.getContainerStatus(podName, containerIdx)
		if err != nil {
			return false, fmt.Errorf("Error getting container status to check completion: %w", err)
		}

		if status.Image == executor.conf.Images.Pause {
			// still running the pause image - container hasn't restarted with the step image yet
			return false, nil
		}

		if status.State.Terminated != nil {
			exitCode := status.State.Terminated.ExitCode
			if exitCode != 0 {
				return false, fmt.Errorf("Container exited with a non-zero exit code: %d", exitCode)
			} else {
				return true, nil
			}
		}

		return false, nil
	})
}

func (executor *KubernetesExecutor) tailContainerLogs(podName string, containerIdx int) error {
	// wait for the container to be running or terminated first - tailing the logs while it is waiting to start results in an error
	err := wait.PollUntilContextTimeout(k8sExecutorContext, 1*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		status, err := executor.getContainerStatus(podName, containerIdx)
		if err != nil {
			return false, fmt.Errorf("Error getting container status before tailing logs: %w", err)
		}

		if status.Image == executor.conf.Images.Pause {
			// still running the pause image - container hasn't restarted with the step image yet
			return false, nil
		} else if status.State.Terminated == nil && status.State.Running == nil {
			// container is still waiting to start
			return false, nil
		} else {
			return true, nil
		}
	})
	if err != nil {
		return fmt.Errorf("Failed to wait for pod to start before tailing logs: %w", err)
	}

	pod, err := executor.podClient.Get(k8sExecutorContext, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("Failed to get pod before tailing logs: %w", err)
	}

	var sinceSeconds int64 = 60
	logReq := executor.podClient.GetLogs(podName, &v1.PodLogOptions{
		Container:    pod.Spec.Containers[containerIdx].Name,
		Follow:       true,
		SinceSeconds: &sinceSeconds,
	})

	err = logReq.Error()
	if err != nil {
		return fmt.Errorf("Failed to request container logs: %w", err)
	}

	stream, err := logReq.Stream(k8sExecutorContext)
	if err != nil {
		return fmt.Errorf("Failed to follow container logs: %w", err)
	}

	defer stream.Close()
	_, err = io.Copy(os.Stdout, stream)
	if err != nil {
		return fmt.Errorf("Failed to follow container logs: %w", err)
	} else {
		return nil
	}
}
