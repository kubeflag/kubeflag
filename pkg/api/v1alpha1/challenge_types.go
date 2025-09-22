/*
Copyright 2025.

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

// ChallengeSpec defines the desired state of Challenge
// +kubebuilder:object:generate=true
type ChallengeSpec struct {
	Template   DeploymentTemplate `json:"template"`
	DefaultTTL *metav1.Duration   `json:"defaultTTL,omitempty"`
	// Optional field to specify the name of the container to expose
	ExposedContainerName string `json:"exposedContainerName,omitempty"`
	// List of Secrets referenced by the Challenge
	SecretReferences []corev1.ObjectReference `json:"secretReferences,omitempty"`

	// List of ConfigMaps referenced by the Challenge
	ConfigMapReferences []corev1.ObjectReference `json:"configMapReferences,omitempty"`
}

// +kubebuilder:object:generate=true
type DeploymentTemplate struct {
	Spec corev1.PodSpec `json:"spec"`
}

// ChallengeStatus defines the observed state of Challenge
// +kubebuilder:object:generate=true
// +kubebuilder:subresource:status
type ChallengeStatus struct {
	Healthy         bool        `json:"healthy"`
	TemplateHash    string      `json:"templateHash"`
	ActiveInstances int         `json:"activeInstances"`
	LastUpdated     metav1.Time `json:"lastUpdated,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Healthy",type=string,JSONPath=`.status.healthy`,description="Health status of the challenge"
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.spec.defaultTTL`,description="Default duration for the challenge instance"
// +kubebuilder:printcolumn:name="Instances",type=integer,JSONPath=`.status.activeInstances`,description="Number of active instances"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Challenge is the Schema for the challenges API
// +kubebuilder:resource:scope=Cluster
type Challenge struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Challenge
	// +required
	Spec ChallengeSpec `json:"spec"`

	// status defines the observed state of Challenge
	// +optional
	Status ChallengeStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// ChallengeList contains a list of Challenge
type ChallengeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Challenge `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Challenge{}, &ChallengeList{})
}
