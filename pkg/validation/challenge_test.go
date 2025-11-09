package validation

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

func TestValidateContainers(t *testing.T) {
	path := field.NewPath("spec", "template", "spec", "containers")

	t.Run("should fail when no containers provided", func(t *testing.T) {
		errs := ValidateContainers(nil, path)
		if len(errs) == 0 {
			t.Errorf("expected error for empty container list, got none")
		}
	})

	t.Run("should fail when container name is missing", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Image: "nginx:alpine",
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{},
					Requests: corev1.ResourceList{},
				},
			},
		}

		errs := ValidateContainers(containers, path)
		if len(errs) == 0 {
			t.Errorf("expected error for missing name, got none")
		}
	})

	t.Run("should fail when container image is missing", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name: "nginx",
				Resources: corev1.ResourceRequirements{
					Limits:   corev1.ResourceList{},
					Requests: corev1.ResourceList{},
				},
			},
		}

		errs := ValidateContainers(containers, path)
		if len(errs) == 0 {
			t.Errorf("expected error for missing image, got none")
		}
	})

	t.Run("should fail when resources are missing", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name:  "nginx",
				Image: "nginx:alpine",
			},
		}

		errs := ValidateContainers(containers, path)
		if len(errs) == 0 {
			t.Errorf("expected error for missing resource requests/limits, got none")
		}
	})

	t.Run("should pass with valid container", func(t *testing.T) {
		containers := []corev1.Container{
			{
				Name:  "nginx",
				Image: "nginx:alpine",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("250m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
			},
		}

		errs := ValidateContainers(containers, path)
		if len(errs) != 0 {
			t.Errorf("expected no validation errors, got: %v", errs.ToAggregate())
		}
	})
}
