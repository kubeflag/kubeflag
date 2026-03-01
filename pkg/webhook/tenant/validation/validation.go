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

// Add registers the Tenant validation webhook with the manager.
func Add(mgr manager.Manager, log logr.Logger) error {
	if err := builder.WebhookManagedBy(mgr).
		For(&kubeflagv1.Tenant{}).
		WithValidator(NewValidator(mgr.GetClient())).
		Complete(); err != nil {
		log.Error(err, "Failed to setup Tenant validation webhook")
		return err
	}

	log.Info("Tenant validation webhook registered")
	return nil
}

func (v *validator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	tenant, ok := obj.(*kubeflagv1.Tenant)
	if !ok {
		return nil, errors.New("object is not a Tenant")
	}

	var allErrs field.ErrorList

	// Name must be a valid DNS label (RFC 1035).
	if errs := k8svalidation.IsDNS1035Label(tenant.Name); len(errs) != 0 {
		return nil, fmt.Errorf("tenant name must be a valid RFC 1035 label: %s", strings.Join(errs, ", "))
	}

	// Validate rejectedConsumers.
	if errs := v.validateRejectedConsumers(ctx, tenant.Spec.RejectedConsumers); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	newTenant, ok := newObj.(*kubeflagv1.Tenant)
	if !ok {
		return nil, errors.New("new object is not a Tenant")
	}

	var allErrs field.ErrorList

	// Validate rejectedConsumers.
	if errs := v.validateRejectedConsumers(ctx, newTenant.Spec.RejectedConsumers); len(errs) > 0 {
		allErrs = append(allErrs, errs...)
	}

	return nil, allErrs.ToAggregate()
}

func (v *validator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateRejectedConsumers checks that entries are non-empty, unique, and reference existing Consumers.
func (v *validator) validateRejectedConsumers(ctx context.Context, consumers []string) field.ErrorList {
	var allErrs field.ErrorList
	basePath := field.NewPath("spec", "rejectedConsumers")

	seen := make(map[string]struct{})
	for i, name := range consumers {
		idxPath := basePath.Index(i)

		// Must not be empty.
		if name == "" {
			allErrs = append(allErrs, field.Required(idxPath, "consumer name must not be empty"))
			continue
		}

		// Must not be duplicate.
		if _, exists := seen[name]; exists {
			allErrs = append(allErrs, field.Duplicate(idxPath, name))
			continue
		}
		seen[name] = struct{}{}

		// Must reference an existing Consumer.
		consumer := &kubeflagv1.Consumer{}
		err := v.client.Get(ctx, types.NamespacedName{Name: name}, consumer)
		if err != nil {
			if apierrors.IsNotFound(err) {
				allErrs = append(allErrs, field.NotFound(idxPath, fmt.Sprintf("Consumer %q does not exist", name)))
			} else {
				allErrs = append(allErrs, field.InternalError(idxPath, fmt.Errorf("error looking up Consumer %q: %w", name, err)))
			}
		}
	}

	return allErrs
}
