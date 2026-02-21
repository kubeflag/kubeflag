/*
Copyright 2026 The KubeFlag Authors.

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
	"crypto/x509"
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	"github.com/kubeflag/kubeflag/pkg/validation"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// validator for validating Kubeflag Challenge CRD.
type validator struct {
	client           ctrlruntimeclient.Client
	caBundle         *x509.CertPool
	SecretsGetter    func(kubeflagv1.DeploymentTemplate) []string
	ConfigmapsGetter func(kubeflagv1.DeploymentTemplate) []string
}

// NewValidator returns a new challenge validator.
func NewValidator(client ctrlruntimeclient.Client, caBundle *x509.CertPool) *validator {
	return &validator{
		client:           client,
		caBundle:         caBundle,
		SecretsGetter:    getSecretsFromChallengeTemplate,
		ConfigmapsGetter: getConfigMapsFromChallengeTemplate,
	}
}

var _ admission.CustomValidator = &validator{}

// Add registers the Challenge validation webhook with the given manager.
func Add(mgr manager.Manager, log logr.Logger, caPool *x509.CertPool) error {
	validator := NewValidator(mgr.GetClient(), caPool)

	if err := builder.WebhookManagedBy(mgr).
		For(&kubeflagv1.Challenge{}).
		WithValidator(validator).
		Complete(); err != nil {
		log.Error(err, "Failed to setup Challenge validation webhook")
		return err
	}

	log.Info("Challenge validation webhook registered")
	return nil
}

func (v *validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList

	challenge, ok := obj.(*kubeflagv1.Challenge)
	if !ok {
		return nil, errors.New("object is not a Challenge")
	}

	// This validates the charset and the max length.
	if errs := k8svalidation.IsDNS1035Label(challenge.Name); len(errs) != 0 {
		return nil, fmt.Errorf("challenge name must be valid rfc1035 label: %s", strings.Join(errs, ","))
	}
	if len(challenge.Name) > validation.MaxChallengeNameLength {
		return nil, fmt.Errorf("challenge name exceeds maximum allowed length of %d characters", validation.MaxChallengeNameLength)
	}

	if errs := validation.ValidateNewChallengeSpec(ctx, &challenge.Spec, v.SecretsGetter, v.ConfigmapsGetter, nil); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate referenced objects (Secrets and ConfigMaps)
	if errs := v.validateReferencedObjects(ctx, &challenge.Spec); errs != nil {
		allErrs = append(allErrs, errs...)
	}
	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList

	oldChallenge, ok := oldObj.(*kubeflagv1.Challenge)
	if !ok {
		return nil, errors.New("old object is not a Challenge")
	}

	newChallenge, ok := newObj.(*kubeflagv1.Challenge)
	if !ok {
		return nil, errors.New("new object is not a challenge")
	}

	if errs := validation.ValidateChallengeUpdate(ctx, newChallenge, oldChallenge, v.SecretsGetter, v.ConfigmapsGetter, nil); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	// Validate referenced objects (Secrets and ConfigMaps)
	if errs := v.validateReferencedObjects(ctx, &newChallenge.Spec); errs != nil {
		allErrs = append(allErrs, errs...)
	}

	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateDelete(ctx context.Context, oldObj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
func (v *validator) validateReferencedObjects(ctx context.Context, spec *kubeflagv1.ChallengeSpec) field.ErrorList {
	var allErrs field.ErrorList

	// Validate SecretReferences
	for _, secretRef := range spec.SecretReferences {
		secret := &corev1.Secret{}
		err := v.client.Get(ctx, ctrlruntimeclient.ObjectKey{Namespace: secretRef.Namespace, Name: secretRef.Name}, secret)
		if err != nil {
			if apierrors.IsNotFound(err) {
				allErrs = append(allErrs, field.NotFound(field.NewPath("spec", "secretReferences"), fmt.Sprintf("Secret %s/%s not found", secretRef.Namespace, secretRef.Name)))
			} else {
				allErrs = append(allErrs, field.InternalError(field.NewPath("spec", "secretReferences"), fmt.Errorf("error fetching Secret %s/%s: %w", secretRef.Namespace, secretRef.Name, err)))
			}
		}
	}

	// Validate ConfigMapReferences
	for _, configMapRef := range spec.ConfigMapReferences {
		configMap := &corev1.ConfigMap{}
		err := v.client.Get(ctx, ctrlruntimeclient.ObjectKey{Namespace: configMapRef.Namespace, Name: configMapRef.Name}, configMap)
		if err != nil {
			if apierrors.IsNotFound(err) {
				allErrs = append(allErrs, field.NotFound(field.NewPath("spec", "configMapReferences"), fmt.Sprintf("ConfigMap %s/%s not found", configMapRef.Namespace, configMapRef.Name)))
			} else {
				allErrs = append(allErrs, field.InternalError(field.NewPath("spec", "configMapReferences"), fmt.Errorf("error fetching ConfigMap %s/%s: %w", configMapRef.Namespace, configMapRef.Name, err)))
			}
		}
	}

	return allErrs
}

// getSecretsFromChallengeTemplate returns a slice of all Secret names referenced in a PodSpec.
func getSecretsFromChallengeTemplate(template kubeflagv1.DeploymentTemplate) []string {
	podSpec := template.Spec
	secretSet := make(map[string]struct{}) // Use a set to avoid duplicates
	secrets := []string{}

	// Check environment variables
	for _, container := range append(podSpec.Containers, podSpec.InitContainers...) {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				secretSet[env.ValueFrom.SecretKeyRef.Name] = struct{}{}
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.SecretRef != nil {
				secretSet[envFrom.SecretRef.Name] = struct{}{}
			}
		}
	}

	// Check volumes
	for _, volume := range podSpec.Volumes {
		if volume.Secret != nil {
			secretSet[volume.Secret.SecretName] = struct{}{}
		}
	}

	// Check image pull secrets
	for _, imagePullSecret := range podSpec.ImagePullSecrets {
		secretSet[imagePullSecret.Name] = struct{}{}
	}

	// Convert set to slice
	for secret := range secretSet {
		secrets = append(secrets, secret)
	}

	return secrets
}

// getConfigMapsFromChallengeTemplate returns a slice of all ConfigMap names referenced in a PodSpec.
func getConfigMapsFromChallengeTemplate(template kubeflagv1.DeploymentTemplate) []string {
	podSpec := template.Spec
	configMapSet := make(map[string]struct{}) // Use a set to avoid duplicates
	configMaps := []string{}

	// Check environment variables
	for _, container := range append(podSpec.Containers, podSpec.InitContainers...) {
		for _, env := range container.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				configMapSet[env.ValueFrom.ConfigMapKeyRef.Name] = struct{}{}
			}
		}
		for _, envFrom := range container.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				configMapSet[envFrom.ConfigMapRef.Name] = struct{}{}
			}
		}
	}

	// Check volumes
	for _, volume := range podSpec.Volumes {
		if volume.ConfigMap != nil {
			configMapSet[volume.ConfigMap.Name] = struct{}{}
		}
	}

	// Convert set to slice
	for configMap := range configMapSet {
		configMaps = append(configMaps, configMap)
	}

	return configMaps
}
