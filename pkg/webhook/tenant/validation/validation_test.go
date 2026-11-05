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
	"testing"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntimefakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = kubeflagv1.AddToScheme(scheme)
	return scheme
}

func existingConsumer(name string) *kubeflagv1.Consumer {
	return &kubeflagv1.Consumer{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
}

func TestValidateCreate(t *testing.T) {
	tests := []struct {
		name      string
		tenant    *kubeflagv1.Tenant
		objects   []runtime.Object
		expectErr bool
	}{
		{
			name: "valid tenant, no rejectedConsumers",
			tenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec: kubeflagv1.TenantSpec{
					Policies: kubeflagv1.TenantPolicies{
						MaxInstancesPerUser: 5,
						MaxInstancesPerTeam: 10,
					},
				},
			},
			objects:   []runtime.Object{},
			expectErr: false,
		},
		{
			name: "valid tenant with existing rejectedConsumers",
			tenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec: kubeflagv1.TenantSpec{
					RejectedConsumers: []string{"bad-consumer"},
				},
			},
			objects:   []runtime.Object{existingConsumer("bad-consumer")},
			expectErr: false,
		},
		{
			name: "invalid name (not DNS-compatible)",
			tenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "INVALID_NAME!"},
				Spec:       kubeflagv1.TenantSpec{},
			},
			objects:   []runtime.Object{},
			expectErr: true,
		},
		{
			name: "rejectedConsumer references non-existent Consumer",
			tenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec: kubeflagv1.TenantSpec{
					RejectedConsumers: []string{"does-not-exist"},
				},
			},
			objects:   []runtime.Object{},
			expectErr: true,
		},
		{
			name: "duplicate entries in rejectedConsumers",
			tenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec: kubeflagv1.TenantSpec{
					RejectedConsumers: []string{"consumer-a", "consumer-a"},
				},
			},
			objects:   []runtime.Object{existingConsumer("consumer-a")},
			expectErr: true,
		},
		{
			name: "empty string in rejectedConsumers",
			tenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec: kubeflagv1.TenantSpec{
					RejectedConsumers: []string{""},
				},
			},
			objects:   []runtime.Object{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme()
			clientBuilder := ctrlruntimefakeclient.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				clientBuilder = clientBuilder.WithRuntimeObjects(obj)
			}
			client := clientBuilder.Build()

			v := NewValidator(client)
			_, err := v.ValidateCreate(context.Background(), tt.tenant)

			if tt.expectErr && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestValidateUpdate(t *testing.T) {
	tests := []struct {
		name      string
		oldTenant *kubeflagv1.Tenant
		newTenant *kubeflagv1.Tenant
		objects   []runtime.Object
		expectErr bool
	}{
		{
			name: "no changes",
			oldTenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec:       kubeflagv1.TenantSpec{},
			},
			newTenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec:       kubeflagv1.TenantSpec{},
			},
			objects:   []runtime.Object{},
			expectErr: false,
		},
		{
			name: "rejectedConsumer references non-existent Consumer on update",
			oldTenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec:       kubeflagv1.TenantSpec{},
			},
			newTenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec: kubeflagv1.TenantSpec{
					RejectedConsumers: []string{"ghost"},
				},
			},
			objects:   []runtime.Object{},
			expectErr: true,
		},
		{
			name: "duplicate entries in rejectedConsumers on update",
			oldTenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec:       kubeflagv1.TenantSpec{},
			},
			newTenant: &kubeflagv1.Tenant{
				ObjectMeta: metav1.ObjectMeta{Name: "my-tenant"},
				Spec: kubeflagv1.TenantSpec{
					RejectedConsumers: []string{"consumer-a", "consumer-a"},
				},
			},
			objects:   []runtime.Object{existingConsumer("consumer-a")},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme()
			clientBuilder := ctrlruntimefakeclient.NewClientBuilder().WithScheme(scheme)
			for _, obj := range tt.objects {
				clientBuilder = clientBuilder.WithRuntimeObjects(obj)
			}
			client := clientBuilder.Build()

			v := NewValidator(client)
			_, err := v.ValidateUpdate(context.Background(), tt.oldTenant, tt.newTenant)

			if tt.expectErr && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}
