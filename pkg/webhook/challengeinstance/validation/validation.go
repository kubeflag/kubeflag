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
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type validator struct {
	client ctrlruntimeclient.Client
}

func NewValidator(client ctrlruntimeclient.Client) *validator {
	return &validator{client: client}
}

var _ admission.CustomValidator = &validator{}

// Add registers the ChallengeInstance validation webhook with the manager.
func Add(mgr manager.Manager, log logr.Logger) error {
	if err := builder.WebhookManagedBy(mgr).
		For(&kubeflagv1.ChallengeInstance{}).
		WithValidator(NewValidator(mgr.GetClient())).
		Complete(); err != nil {
		log.Error(err, "Failed to setup ChallengeInstance validation webhook")
		return err
	}

	log.Info("ChallengeInstance validation webhook registered")
	return nil
}

func (v *validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	instance, ok := obj.(*kubeflagv1.ChallengeInstance)
	if !ok {
		return nil, errors.New("object is not a ChallengeInstance")
	}

	var allErrs field.ErrorList

	challengeRefPath := field.NewPath("spec", "challengeRef")
	userPath := field.NewPath("spec", "user")
	ttlPath := field.NewPath("spec", "ttl")

	// challengeRef must not be empty.
	if instance.Spec.ChallengeRef == "" {
		allErrs = append(allErrs, field.Required(challengeRefPath, "challengeRef must be set"))
		return nil, allErrs.ToAggregate()
	}

	// challengeRef must reference an existing Challenge that is healthy.
	if err := v.validateChallengeRef(ctx, instance.Spec.ChallengeRef, challengeRefPath, &allErrs); err != nil {
		return nil, err
	}

	// user must not be empty.
	if instance.Spec.User == "" {
		allErrs = append(allErrs, field.Required(userPath, "user must be set"))
	} else {
		// user must be DNS-compatible (combined <challenge>-<user> becomes a Deployment name).
		if errs := k8svalidation.IsDNS1123Label(instance.Spec.User); len(errs) != 0 {
			allErrs = append(allErrs, field.Invalid(
				userPath,
				instance.Spec.User,
				fmt.Sprintf("user must be a valid DNS label: %s", strings.Join(errs, ", ")),
			))
		}
	}

	// ttl, if set, must be positive.
	if instance.Spec.TTL != nil && instance.Spec.TTL.Duration <= 0 {
		allErrs = append(allErrs, field.Invalid(
			ttlPath,
			instance.Spec.TTL.Duration.String(),
			"ttl must be a positive duration",
		))
	}

	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldInstance, ok := oldObj.(*kubeflagv1.ChallengeInstance)
	if !ok {
		return nil, errors.New("old object is not a ChallengeInstance")
	}
	newInstance, ok := newObj.(*kubeflagv1.ChallengeInstance)
	if !ok {
		return nil, errors.New("new object is not a ChallengeInstance")
	}

	var allErrs field.ErrorList

	challengeRefPath := field.NewPath("spec", "challengeRef")
	userPath := field.NewPath("spec", "user")

	// challengeRef is immutable after creation.
	if oldInstance.Spec.ChallengeRef != newInstance.Spec.ChallengeRef {
		allErrs = append(allErrs, field.Forbidden(
			challengeRefPath,
			fmt.Sprintf("challengeRef is immutable: cannot change from %q to %q",
				oldInstance.Spec.ChallengeRef, newInstance.Spec.ChallengeRef),
		))
	}

	// user is immutable after creation.
	if oldInstance.Spec.User != newInstance.Spec.User {
		allErrs = append(allErrs, field.Forbidden(
			userPath,
			fmt.Sprintf("user is immutable: cannot change from %q to %q",
				oldInstance.Spec.User, newInstance.Spec.User),
		))
	}

	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateChallengeRef checks that the referenced Challenge exists and is healthy.
// Returns a non-nil error only when there is an unexpected internal failure.
func (v *validator) validateChallengeRef(ctx context.Context, challengeRef string, path *field.Path, allErrs *field.ErrorList) error {
	challenge := &kubeflagv1.Challenge{}
	err := v.client.Get(ctx, types.NamespacedName{Name: challengeRef}, challenge)
	if err != nil {
		if apierrors.IsNotFound(err) {
			*allErrs = append(*allErrs, field.NotFound(path, fmt.Sprintf("Challenge %q does not exist", challengeRef)))
			return nil
		}
		return fmt.Errorf("error looking up Challenge %q: %w", challengeRef, err)
	}

	if !challenge.Status.Healthy {
		*allErrs = append(*allErrs, field.Forbidden(path, fmt.Sprintf("Challenge %q is not healthy", challengeRef)))
	}

	return nil
}
