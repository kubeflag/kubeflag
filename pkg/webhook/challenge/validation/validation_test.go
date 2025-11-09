package validation

import (
	"reflect"
	"testing"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
)

func TestGetSecretsFromPodSpec(t *testing.T) {
	// Define the input PodSpec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Env: []corev1.EnvVar{
					{
						Name: "SECRET_VAR",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
								Key:                  "key1",
							},
						},
					},
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "secret-volume",
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: "volume-secret",
					},
				},
			},
		},
		ImagePullSecrets: []corev1.LocalObjectReference{
			{Name: "pull-secret"},
		},
	}

	template := kubeflagv1.DeploymentTemplate{Spec: podSpec}

	// Expected output
	expectedSecrets := []string{"my-secret", "volume-secret", "pull-secret"}

	// Call the function
	actualSecrets := getSecretsFromChallengeTemplate(template)

	// Sort slices for comparison (optional)
	expectedSecretsMap := make(map[string]struct{})
	actualSecretsMap := make(map[string]struct{})

	for _, secret := range expectedSecrets {
		expectedSecretsMap[secret] = struct{}{}
	}
	for _, secret := range actualSecrets {
		actualSecretsMap[secret] = struct{}{}
	}

	// Assert the results
	if !reflect.DeepEqual(expectedSecretsMap, actualSecretsMap) {
		t.Errorf("Expected secrets: %v, but got: %v", expectedSecrets, actualSecrets)
	}
}

func TestGetConfigMapsFromPodSpec(t *testing.T) {
	// Define the input PodSpec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			{
				Env: []corev1.EnvVar{
					{
						Name: "CONFIG_VAR",
						ValueFrom: &corev1.EnvVarSource{
							ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "my-configmap"},
								Key:                  "key1",
							},
						},
					},
				},
			},
		},
		Volumes: []corev1.Volume{
			{
				Name: "config-volume",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "volume-configmap"},
					},
				},
			},
		},
	}

	template := kubeflagv1.DeploymentTemplate{Spec: podSpec}

	// Expected output
	expectedConfigMaps := []string{"my-configmap", "volume-configmap"}

	// Call the function
	actualConfigMaps := getConfigMapsFromChallengeTemplate(template)

	// Sort slices for comparison (optional)
	expectedConfigMapsMap := make(map[string]struct{})
	actualConfigMapsMap := make(map[string]struct{})

	for _, cm := range expectedConfigMaps {
		expectedConfigMapsMap[cm] = struct{}{}
	}
	for _, cm := range actualConfigMaps {
		actualConfigMapsMap[cm] = struct{}{}
	}

	// Assert the results
	if !reflect.DeepEqual(expectedConfigMapsMap, actualConfigMapsMap) {
		t.Errorf("Expected ConfigMaps: %v, but got: %v", expectedConfigMaps, actualConfigMaps)
	}
}
