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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:generate=true
// TenantSpec defines the desired state of Tenant.
type TenantSpec struct {
	// Display name for human-readability
	// +optional
	DisplayName string `json:"displayName,omitempty"`

	// Human-readable description
	// +optional
	Description string `json:"description,omitempty"`

	// Policies that govern instance creation and lifecycle for this tenant.
	// +required
	Policies TenantPolicies `json:"policies"`

	// List of consumer names that are explicitly rejected from creating instances
	// under this tenant. The consumer names correspond to the `Consumer.metadata.name`.
	// If empty, no consumers are rejected by this tenant (rejection is tenant-local).
	// +optional
	RejectedConsumers []string `json:"rejectedConsumers,omitempty"`
}

// TenantPolicies groups tenant-level policy limitations.
type TenantPolicies struct {
	// Maximum number of simultaneously running instances a single user can have.
	// Set to 0 to disallow instance creation by a single user.
	// +kubebuilder:validation:Minimum=0
	MaxInstancesPerUser int32 `json:"maxInstancesPerUser"`

	// Maximum number of simultaneously running instances a team can have.
	// Set to 0 to disallow instance creation by teams.
	// +kubebuilder:validation:Minimum=0
	MaxInstancesPerTeam int32 `json:"maxInstancesPerTeam"`
}

// TenantStatus defines observed state for Tenant (informational; updated by controllers).
type TenantStatus struct {
	// Total number of ChallengeInstances currently active for this Tenant.
	// +optional
	TotalInstances int32 `json:"totalInstances,omitempty"`

	// Last time the controller updated the status for this tenant.
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=tn
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="MaxPerUser",type=integer,JSONPath=`.spec.policies.maxInstancesPerUser`
// +kubebuilder:printcolumn:name="MaxPerTeam",type=integer,JSONPath=`.spec.policies.maxInstancesPerTeam`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Tenant struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   TenantSpec   `json:"spec,omitempty"`
	Status TenantStatus `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Tenant{}, &TenantList{})
}
