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

	if containerIdx < len(pod.Status.ContainerStatuses) {
		return nil, nil
	}

	return &pod.Status.ContainerStatuses[containerIdx], nil
}

func (executor *KubernetesExecutor) waitForContainerCompletion(podName string, containerIdx int) (int, error) {
	var exitCode int32

	err := wait.PollUntilContextTimeout(k8sExecutorContext, 2*time.Second, 5*time.Minute, false, func(ctx context.Context) (bool, error) {
		status, err := executor.getContainerStatus(podName, containerIdx)
		if err != nil {
			return true, fmt.Errorf("error getting container status to check completion: %w", err)
		}

		if status == nil {
			// no status reported yet
			return false, nil
		}

		if status.Image == executor.conf.Images.Pause {
			// still running the pause image - container hasn't restarted with the step image yet
			return false, nil
		}

		if status.State.Terminated != nil {
			exitCode = status.State.Terminated.ExitCode
			return true, nil
		}

		return false, nil
	})
	if err != nil {
		return -1, fmt.Errorf("error waiting for pod to complete: %w", err)
	}

	return int(exitCode), nil
}

func (executor *KubernetesExecutor) printContainerLogs(podName string, containerIdx int) error {
	pod, err := executor.podClient.Get(k8sExecutorContext, podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get pod to print logs: %w", err)
	}

	logReq := executor.podClient.GetLogs(podName, &v1.PodLogOptions{
		Container: pod.Spec.Containers[containerIdx].Name,
	})

	err = logReq.Error()
	if err != nil {
		return fmt.Errorf("failed to request container logs: %w", err)
	}

	stream, err := logReq.Stream(k8sExecutorContext)
	if err != nil {
		return fmt.Errorf("failed opening stream for container logs: %w", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	_, err = io.Copy(os.Stdout, stream)
	if err != nil {
		return fmt.Errorf("failed to stream container logs: %w", err)
	} else {
		return nil
	}
}
