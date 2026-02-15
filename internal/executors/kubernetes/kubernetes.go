package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/markormesher/tedium/internal/logging"
	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8s "k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var l = logging.Logger
var k8sExecutorContext = context.TODO()

var logPrinterLock sync.Mutex

type KubernetesExecutor struct {
	KubeconfigPath string
	Namespace      string

	// private state
	conf      schema.TediumConfig
	clientSet *k8s.Clientset
	podClient corev1.PodInterface
}

func FromConfig(c schema.KubernetesExecutorConfig) (*KubernetesExecutor, error) {
	namespace := c.Namespace
	if namespace == "" {
		l.Warn("Kubernetes executor namespace was blank - using 'default'")
		namespace = "default"
	}

	return &KubernetesExecutor{
		KubeconfigPath: c.KubeconfigPath,
		Namespace:      namespace,
	}, nil
}

func (executor *KubernetesExecutor) Init(conf schema.TediumConfig) error {
	executor.conf = conf

	var kubeConfig *rest.Config
	var err error

	if executor.KubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", executor.KubeconfigPath)
		if err != nil {
			return fmt.Errorf("error creating Kube config from provided path: %w", err)
		}
	} else {
		l.Info("No kubeconfig path provided - attempting to use in-cluster config")
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("error creating Kube config in-cluster config: %w", err)
		}
	}

	clientSet, err := k8s.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("error creating new Kubernetes client: %w", err)
	}

	executor.clientSet = clientSet
	executor.podClient = clientSet.CoreV1().Pods(executor.Namespace)

	return nil
}

func (executor *KubernetesExecutor) ExecuteChore(job schema.Job) error {
	// annoying hack so we can pass an *int64 below
	zero := int64(0)

	// configure the execution pod, using the pause image that does nothing in each step for now
	podName := utils.UniqueName("executor")
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: executor.Namespace,
			Name:      podName,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "tedium",
				"app.kubernetes.io/component": "executor",
			},
		},
		Spec: v1.PodSpec{
			RestartPolicy:                 "Never",
			Containers:                    make([]v1.Container, len(job.ExecutionSteps)),
			TerminationGracePeriodSeconds: &zero,
			Volumes: []v1.Volume{
				{
					Name: "repo",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
		},
	}

	for i, step := range job.ExecutionSteps {
		pod.Spec.Containers[i] = v1.Container{
			Name:            step.Label,
			Image:           executor.conf.Images.Pause,
			ImagePullPolicy: "Always",
			Env:             environmentFromMap(step.Environment),
			Command:         []string{"/bin/sh", "-c"},
			Args:            []string{"echo \"${TEDIUM_COMMAND}\" | /bin/sh"},
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "repo",
					MountPath: "/tedium/repo",
				},
			},
		}
	}

	// create the pod and defer its cleanup
	podsClient := executor.clientSet.CoreV1().Pods(executor.Namespace)
	_, err := podsClient.Create(k8sExecutorContext, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error creating execution pod: %w", err)
	}

	defer func() {
		err := executor.podClient.Delete(k8sExecutorContext, podName, metav1.DeleteOptions{})
		if err != nil {
			l.Error("error deleting execution pod", "error", err)
		}
	}()

	// run actual steps by swapping the image on each container within the pod
	for i, step := range job.ExecutionSteps {
		patch := schema.JsonPatch{
			schema.JsonPatchOperation{
				Operation: "replace",
				Path:      fmt.Sprintf("/spec/containers/%d/image", i),
				Value:     step.Image,
			},
		}
		patchBytes, err := json.Marshal(patch)
		l.Info("Starting step", "step", i)
		if err != nil {
			return fmt.Errorf("error marshalling JSON patch to set image: %w", err)
		}

		_, err = podsClient.Patch(k8sExecutorContext, pod.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			return fmt.Errorf("error patching pod to set image: %w", err)
		}

		exitCode, err := executor.waitForContainerCompletion(podName, i)
		if err != nil {
			return fmt.Errorf("error waiting for pod container to complete: %w", err)
		}

		// acquire a lock for output printing, so we don't mingle logs from multiple containers
		logPrinterLock.Lock()
		l.Info("START of logs for container", "pod", podName, "container", i)
		err = executor.printContainerLogs(podName, i)
		if err != nil {
			l.Error("failed to print container logs", "err", err)
		}
		l.Info("END of logs for container", "pod", podName, "container", i)
		logPrinterLock.Unlock()

		if exitCode != 0 {
			return fmt.Errorf("container finished with a non-zero exit code: %d", exitCode)
		}
	}

	return nil
}
