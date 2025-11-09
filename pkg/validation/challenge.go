/*
Copyright 2025 The KubeFlag Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package validation

import (
	"context"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

const (
	// MaxChallengeNameLength is the maximum allowed length for challenge names.
	MaxChallengeNameLength = 36
)

type dataGetter func(kubeflagv1.DeploymentTemplate) []string

// ValidateNewChallengeSpec validates the given challenge spec. If this is not called from within another validation
// routine, parentFieldPath can be nil.
func ValidateNewChallengeSpec(ctx context.Context, spec *kubeflagv1.ChallengeSpec, secretsGetter, configmapsGetter dataGetter, parentFieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate the template
	if errs := ValidateChallengeSpec(spec, parentFieldPath); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate references
	if errs := ValidateReferences(spec, secretsGetter, configmapsGetter, parentFieldPath); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}
	return allErrs
}

// ValidateChallengeUpdate validates the new challenge and if no forbidden changes were attempted.
func ValidateChallengeUpdate(ctx context.Context, newChallenge, oldChallenge *kubeflagv1.Challenge, secretsGetter, configmapsGetter dataGetter, parentFieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Validate the template
	if errs := ValidateChallengeSpec(&newChallenge.Spec, parentFieldPath); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate references
	if errs := ValidateReferences(&newChallenge.Spec, secretsGetter, configmapsGetter, parentFieldPath); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}
	return allErrs
}

func ValidateReferences(spec *kubeflagv1.ChallengeSpec, secretsGetter, configmapsGetter dataGetter, parentFieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	// Extract secrets and configmaps from the template PodSpec
	usedSecrets := secretsGetter(spec.Template)
	usedConfigMaps := configmapsGetter(spec.Template)

	// Validate secrets
	referencedSecrets := map[string]struct{}{}
	for _, ref := range spec.SecretReferences {
		referencedSecrets[ref.Name] = struct{}{}
	}

	for _, secret := range usedSecrets {
		if _, found := referencedSecrets[secret]; !found {
			allErrs = append(allErrs, field.NotFound(parentFieldPath.Child("secretReferences"), secret))
		}
	}

	// Validate configmaps
	referencedConfigMaps := map[string]struct{}{}
	for _, ref := range spec.ConfigMapReferences {
		referencedConfigMaps[ref.Name] = struct{}{}
	}

	for _, configMap := range usedConfigMaps {
		if _, found := referencedConfigMaps[configMap]; !found {
			allErrs = append(allErrs, field.NotFound(parentFieldPath.Child("configMapReferences"), configMap))
		}
	}

	return allErrs
}

// ValidateChallengeSpec validates the given challenge spec. If this is not called from within another validation
// routine, parentFieldPath can be nil.
func ValidateChallengeSpec(spec *kubeflagv1.ChallengeSpec, parentFieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}

	if errs := ValidateTemplate(spec.Template, parentFieldPath.Child("template")); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	return allErrs
}

func ValidateTemplate(template kubeflagv1.DeploymentTemplate, parentFieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if errs := ValidateContainers(template.Spec.Containers, parentFieldPath.Child("containers")); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}
	return allErrs
}

func ValidateContainers(containers []corev1.Container, parentFieldPath *field.Path) field.ErrorList {
	allErrs := field.ErrorList{}
	if len(containers) == 0 {
		allErrs = append(allErrs, field.Required(parentFieldPath, "Deployment template must have at least one container."))
	}

	// Check each container for resource limits and container ports
	for i, container := range containers {
		if container.Name == "" {
			allErrs = append(allErrs, field.Required(parentFieldPath.Index(i).Child("name"), "Deployment template must have at least one container."))
		}

		if container.Image == "" {
			allErrs = append(allErrs, field.Required(parentFieldPath.Index(i).Child("image"), "Container image cannot be empty."))
		}

		if container.Resources.Limits == nil {
			allErrs = append(allErrs, field.Required(parentFieldPath.Index(i).Child("resources").Child("limits"), "Container must have a limits"))
		}

		if container.Resources.Requests == nil {
			allErrs = append(allErrs, field.Required(parentFieldPath.Index(i).Child("resources").Child("requests"), "Container must have a requests"))
		}
	}
	return allErrs
}
