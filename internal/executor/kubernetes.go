package executor

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

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
	conf       schema.TediumConfig
	jobQueue   <-chan schema.Job
	eventQueue chan<- schema.Event

	jobClient batchclients.JobInterface
}

func CreateAndStart(conf schema.TediumConfig, jobQueue <-chan schema.Job, eventQueue chan<- schema.Event) error {
	if conf.Executor.Kubernetes.Namespace == "" {
		slog.Warn("Kubernetes executor namespace was blank - using 'default'")
		conf.Executor.Kubernetes.Namespace = "default"
	}

	e := KubernetesExecutor{
		conf:       conf,
		jobQueue:   jobQueue,
		eventQueue: eventQueue,
	}

	var kubeConfig *rest.Config
	var err error

	if e.conf.Executor.Kubernetes.KubeconfigPath != "" {
		kubeConfig, err = clientcmd.BuildConfigFromFlags("", e.conf.Executor.Kubernetes.KubeconfigPath)
		if err != nil {
			return fmt.Errorf("error creating Kube config from provided path: %w", err)
		}
	} else {
		slog.Info("no kubeconfig path provided - attempting to use in-cluster config")
		kubeConfig, err = rest.InClusterConfig()
		if err != nil {
			return fmt.Errorf("error creating Kube config in-cluster config: %w", err)
		}
	}

	clientSet, err := k8s.NewForConfig(kubeConfig)
	if err != nil {
		return fmt.Errorf("error creating new Kubernetes client: %w", err)
	}

	e.jobClient = clientSet.BatchV1().Jobs(e.conf.Executor.Kubernetes.Namespace)

	// start workers
	workerWg := sync.WaitGroup{}
	for range conf.Executor.ChoreConcurrency {
		workerWg.Go(func() { e.worker() })
	}

	return nil
}

func (e *KubernetesExecutor) worker() {
	for job := range e.jobQueue {
		err := e.executeChore(job)
		if err != nil {
			slog.Error("chore failed", "repo", job.Repo.Name, "chore", job.Chore.Name, "error", err)
			e.eventQueue <- schema.JobFailed
		} else {
			e.eventQueue <- schema.JobSucceeded
		}

		time.Sleep(5 * time.Second)
	}
}

func (e *KubernetesExecutor) executeChore(job schema.Job) error {
	jobName := utils.UniqueName("executor")
	k8sJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: e.conf.Executor.Kubernetes.Namespace,
			Name:      jobName,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            new(int32(0)),
			TTLSecondsAfterFinished: new(int32(5 * 60)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/name":      "tedium",
						"app.kubernetes.io/component": "executor",
					},
				},
				Spec: corev1.PodSpec{
					RestartPolicy:                 corev1.RestartPolicyNever,
					TerminationGracePeriodSeconds: new(int64(0)),
					Containers: []corev1.Container{
						{
							Name:    "finish",
							Image:   e.conf.Images.Tedium,
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

		k8sJob.Spec.Template.Spec.InitContainers = append(k8sJob.Spec.Template.Spec.InitContainers, container)
	}

	// start the job
	_, err := e.jobClient.Create(k8sExecutorContext, k8sJob, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("error creating execution job: %w", err)
	}

	// TODO: track job to completion

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
