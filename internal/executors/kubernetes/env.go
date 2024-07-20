package kubernetes

import v1 "k8s.io/api/core/v1"

func environmentFromMap(mapEnv map[string]string) []v1.EnvVar {
	env := make([]v1.EnvVar, len(mapEnv))
	envCount := 0
	for k, v := range mapEnv {
		env[envCount] = v1.EnvVar{
			Name:  k,
			Value: v,
		}
		envCount++
	}
	return env
}
