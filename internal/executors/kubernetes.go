package executors

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/markormesher/tedium/internal/schema"
	"github.com/markormesher/tedium/internal/utils"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	batchclients "k8s.io/client-go/kubernetes/typed/batch/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var k8sExecutorContext = context.TODO()

type KubernetesExecutor struct {
	KubeconfigPath string
	Namespace      string

	// private state
	conf      schema.TediumConfig
	jobClient batchclients.JobInterface
}

func FromConfig(c schema.ExecutorConfig) (*KubernetesExecutor, error) {
	namespace := c.Kubernetes.Namespace
	if namespace == "" {
		slog.Warn("Kubernetes executor namespace was blank - using 'default'")
		namespace = "default"
	}

	return &KubernetesExecutor{
		KubeconfigPath: c.Kubernetes.KubeconfigPath,
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
		slog.Info("No kubeconfig path provided - attempting to use in-cluster config")
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("error creating Kube config in-cluster config: %w", err)
		}
	}

	clientSet, err := k8s.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("error creating new Kubernetes client: %w", err)
	}

	executor.jobClient = clientSet.BatchV1().Jobs(executor.Namespace)

	return nil
}

func (executor *KubernetesExecutor) ExecuteChore(job schema.Job) error {
	jobName := utils.UniqueName("executor")
	pod := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: executor.Namespace,
			Name:      jobName,
			Labels: map[string]string{
				"app.kubernetes.io/name":      "tedium",
				"app.kubernetes.io/component": "executor",
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            new(int32(0)),
			TTLSecondsAfterFinished: new(int32(5 * 60)),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					TerminationGracePeriodSeconds: new(int64(0)),
					Containers: []corev1.Container{
						{
							Name:    "finish",
							Image:   executor.conf.Images.Tedium,
							Command: []string{"exit", "0"},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "repo",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	for _, step := range job.ExecutionSteps {
		container := corev1.Container{
			Name:    step.Label,
			Image:   step.Image,
			Env:     k8sEnvFromMap(step.Environment),
			Command: []string{"/bin/sh", "-c"},
			Args:    []string{"echo \"${TEDIUM_COMMAND}\" | /bin/sh"},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      "repo",
					MountPath: "/tedium/repo",
				},
			},
		}

		pod.Spec.Template.Spec.InitContainers = append(pod.Spec.Template.Spec.InitContainers, container)
	}

	// start the job
	_, err := executor.jobClient.Create(k8sExecutorContext, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error creating execution job: %w", err)
	}

	return nil
}

func k8sEnvFromMap(mapEnv map[string]string) []corev1.EnvVar {
	env := make([]corev1.EnvVar, len(mapEnv))
	envCount := 0
	for k, v := range mapEnv {
		env[envCount] = corev1.EnvVar{
			Name:  k,
			Value: v,
		}
		envCount++
	}
	return env
}
