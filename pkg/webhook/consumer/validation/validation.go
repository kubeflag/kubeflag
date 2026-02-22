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
	"time"

	"github.com/go-logr/logr"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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

// Add registers the Consumer validation webhook with the manager.
func Add(mgr manager.Manager, log logr.Logger) error {
	if err := builder.WebhookManagedBy(mgr).
		For(&kubeflagv1.Consumer{}).
		WithValidator(NewValidator(mgr.GetClient())).
		Complete(); err != nil {
		log.Error(err, "Failed to setup Consumer validation webhook")
		return err
	}

	log.Info("Consumer validation webhook registered")
	return nil
}

func (v *validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	consumer, ok := obj.(*kubeflagv1.Consumer)
	if !ok {
		return nil, errors.New("object is not a Consumer")
	}

	var allErrs field.ErrorList

	tenantRefPath := field.NewPath("spec", "tenantRef")
	expiresAtPath := field.NewPath("spec", "expiresAt")

	// tenantRef must be provided.
	if consumer.Spec.TenantRef == "" {
		allErrs = append(allErrs, field.Required(tenantRefPath, "tenantRef must be set"))
		return nil, allErrs.ToAggregate()
	}

	// Tenant must exist.
	if err := v.validateTenantExists(ctx, consumer.Spec.TenantRef, tenantRefPath, &allErrs); err != nil {
		return nil, err
	}

	// expiresAt must be in the future when provided.
	if consumer.Spec.ExpiresAt != nil && !consumer.Spec.ExpiresAt.After(time.Now()) {
		allErrs = append(allErrs, field.Invalid(
			expiresAtPath,
			consumer.Spec.ExpiresAt,
			"expiresAt must be a future timestamp",
		))
	}

	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldConsumer, ok := oldObj.(*kubeflagv1.Consumer)
	if !ok {
		return nil, errors.New("old object is not a Consumer")
	}
	newConsumer, ok := newObj.(*kubeflagv1.Consumer)
	if !ok {
		return nil, errors.New("new object is not a Consumer")
	}

	var allErrs field.ErrorList

	tenantRefPath := field.NewPath("spec", "tenantRef")
	expiresAtPath := field.NewPath("spec", "expiresAt")

	// tenantRef is immutable after creation.
	if oldConsumer.Spec.TenantRef != newConsumer.Spec.TenantRef {
		allErrs = append(allErrs, field.Forbidden(
			tenantRefPath,
			fmt.Sprintf("tenantRef is immutable: cannot change from %q to %q", oldConsumer.Spec.TenantRef, newConsumer.Spec.TenantRef),
		))
	}

	// expiresAt, when set, must remain in the future.
	if newConsumer.Spec.ExpiresAt != nil && !newConsumer.Spec.ExpiresAt.After(time.Now()) {
		allErrs = append(allErrs, field.Invalid(
			expiresAtPath,
			newConsumer.Spec.ExpiresAt,
			"expiresAt must be a future timestamp",
		))
	}

	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateTenantExists performs the GET against the Tenant and appends to allErrs on failure.
// Returns a non-nil error only when there is an unexpected internal failure.
func (v *validator) validateTenantExists(ctx context.Context, tenantName string, path *field.Path, allErrs *field.ErrorList) error {
	tenant := &kubeflagv1.Tenant{}
	err := v.client.Get(ctx, types.NamespacedName{Name: tenantName}, tenant)
	if err == nil {
		return nil
	}
	if apierrors.IsNotFound(err) {
		*allErrs = append(*allErrs, field.NotFound(path, fmt.Sprintf("Tenant %q does not exist", tenantName)))
		return nil
	}
	return fmt.Errorf("error looking up Tenant %q: %w", tenantName, err)
}
