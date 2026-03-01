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

package mutation

import (
	"context"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Mutator for mutating KubeFlag Tenant CRD.
type Mutator struct{}

// NewMutator returns a new Tenant Mutator.
func NewMutator() *Mutator {
	return &Mutator{}
}

func (m *Mutator) Mutate(_ context.Context, tenant *kubeflagv1.Tenant) (*kubeflagv1.Tenant, *field.Error) {
	// Do not perform mutations on tenants in deletion.
	if tenant.DeletionTimestamp != nil {
		return tenant, nil
	}

	// Default displayName to metadata.name if not set.
	if tenant.Spec.DisplayName == "" {
		tenant.Spec.DisplayName = tenant.Name
	}

	return tenant, nil
}
