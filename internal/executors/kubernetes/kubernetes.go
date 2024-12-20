package kubernetes

import (
	"context"
	"encoding/json"
	"fmt"

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

type KubernetesExecutor struct {
	KubeconfigPath string
	Namespace      string

	// private state
	conf      *schema.TediumConfig
	clientSet *k8s.Clientset
	podClient corev1.PodInterface

	podNames []string
}

func FromConfig(c *schema.KubernetesExecutorConfig) (*KubernetesExecutor, error) {
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

func (executor *KubernetesExecutor) Init(conf *schema.TediumConfig) error {
	executor.conf = conf

	var kubeConfig *rest.Config
	var err error

	if executor.KubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", executor.KubeconfigPath)
		if err != nil {
			return fmt.Errorf("Error creating Kube config from provided path: %w", err)
		}
	} else {
		l.Info("No kubeconfig path provided - attempting to use in-cluster config")
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("Error creating Kube config in-cluster config: %w", err)
		}
	}

	clientSet, err := k8s.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("Error creating new Kubernetes client: %w", err)
	}

	executor.clientSet = clientSet
	executor.podClient = clientSet.CoreV1().Pods(executor.Namespace)

	return nil
}

func (executor *KubernetesExecutor) Deinit() error {
	for _, podName := range executor.podNames {
		err := executor.podClient.Delete(k8sExecutorContext, podName, metav1.DeleteOptions{})
		if err != nil {
			return fmt.Errorf("Error deleting execution pod: %w", err)
		}
	}

	return nil
}

func (executor *KubernetesExecutor) ExecuteChore(job *schema.Job) error {
	totalSteps := len(job.Chore.Steps)

	// annoying hack so we can pass an *int64 below
	var zero int64 = 0

	// setup all the pods we'll need, using the pause image that does nothing

	podName := utils.UniqueName("executor")
	executor.podNames = append(executor.podNames, podName)

	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: executor.Namespace,
			Name:      podName,
		},
		Spec: v1.PodSpec{
			RestartPolicy:                 "Never",
			Containers:                    make([]v1.Container, totalSteps),
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

	for i := range job.ExecutionSteps {
		step := job.ExecutionSteps[i]
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

	podsClient := executor.clientSet.CoreV1().Pods(executor.Namespace)
	_, err := podsClient.Create(k8sExecutorContext, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("Error creating execution pod: %w", err)
	}

	// run actual steps by swapping the image on each container within the pod

	for i := range job.ExecutionSteps {
		step := job.ExecutionSteps[i]
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
			return fmt.Errorf("Error marshalling JSON patch to set image: %w", err)
		}

		_, err = podsClient.Patch(k8sExecutorContext, pod.Name, types.JSONPatchType, patchBytes, metav1.PatchOptions{})
		if err != nil {
			return fmt.Errorf("Error patching pod to set image: %w", err)
		}

		go func() {
			err := executor.tailContainerLogs(podName, i)
			if err != nil {
				l.Error("Failed to tail container logs", "err", err)
			}
		}()

		err = executor.waitForContainerCompletion(podName, i)
		if err != nil {
			return fmt.Errorf("Step failed: %w", err)
		}
	}

	return nil
}
