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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConsumerPhase represents the lifecycle state of a Consumer.
// +kubebuilder:validation:Enum=Active;Suspended;Expired
type ConsumerPhase string

const (
	ConsumerPhaseActive    ConsumerPhase = "Active"
	ConsumerPhaseSuspended ConsumerPhase = "Suspended"
	ConsumerPhaseExpired   ConsumerPhase = "Expired"
)

// ConsumerSpec defines the desired state of Consumer.
type ConsumerSpec struct {
	// TenantRef is the name of the Tenant this consumer belongs to.
	// +required
	TenantRef string `json:"tenantRef"`

	// Human-readable description of this consumer.
	// +optional
	Description string `json:"description,omitempty"`

	// Suspended disables this consumer when set to true.
	// The controller will set the phase to Suspended and stop issuing new tokens.
	// +optional
	Suspended bool `json:"suspended,omitempty"`

	// ExpiresAt is an optional hard expiry time for this consumer.
	// After this time the controller will set the phase to Expired.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
}

// ConsumerStatus defines the observed state of Consumer.
type ConsumerStatus struct {
	// Phase is the current lifecycle state of the consumer.
	// +optional
	Phase ConsumerPhase `json:"phase,omitempty"`

	// TokenSecretRef points to the Secret that holds the issued JWT for this consumer.
	// +optional
	TokenSecretRef *corev1.SecretReference `json:"tokenSecretRef,omitempty"`

	// IssuedAt is the time at which the JWT was first issued.
	// +optional
	IssuedAt *metav1.Time `json:"issuedAt,omitempty"`

	// Conditions holds standard Kubernetes condition entries.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type ConsumerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Consumer `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=co
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Tenant",type=string,JSONPath=`.spec.tenantRef`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type Consumer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	Spec   ConsumerSpec   `json:"spec,omitempty"`
	Status ConsumerStatus `json:"status,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Consumer{}, &ConsumerList{})
}
