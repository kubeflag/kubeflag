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
	"testing"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMutate_DefaultsDisplayName(t *testing.T) {
	tenant := &kubeflagv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
		Spec: kubeflagv1.TenantSpec{
			DisplayName: "", // empty — should be defaulted
		},
	}

	m := NewMutator()
	mutated, err := m.Mutate(context.Background(), tenant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated.Spec.DisplayName != "my-tenant" {
		t.Errorf("expected displayName %q, got %q", "my-tenant", mutated.Spec.DisplayName)
	}
}

func TestMutate_PreservesExistingDisplayName(t *testing.T) {
	tenant := &kubeflagv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
		Spec: kubeflagv1.TenantSpec{
			DisplayName: "My Custom Name",
		},
	}

	m := NewMutator()
	mutated, err := m.Mutate(context.Background(), tenant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated.Spec.DisplayName != "My Custom Name" {
		t.Errorf("expected displayName %q, got %q", "My Custom Name", mutated.Spec.DisplayName)
	}
}

func TestMutate_SkipsWhenDeletionTimestampSet(t *testing.T) {
	now := metav1.Now()
	tenant := &kubeflagv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "my-tenant",
			DeletionTimestamp: &now,
			Finalizers:        []string{"kubeflag.io/cleanup"},
		},
		Spec: kubeflagv1.TenantSpec{
			DisplayName: "", // should NOT be defaulted
		},
	}

	m := NewMutator()
	mutated, err := m.Mutate(context.Background(), tenant)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated.Spec.DisplayName != "" {
		t.Errorf("expected displayName to remain empty for deleting tenant, got %q", mutated.Spec.DisplayName)
	}
}
