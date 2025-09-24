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
type ChallengeInstanceSpec struct {
	ChallengeRef string           `json:"challengeRef"`
	TTL          *metav1.Duration `json:"ttl,omitempty"`
	User         string           `json:"user"`
	Team         string           `json:"team"`
}

// +kubebuilder:object:generate=true
type ChallengeInstanceStatus struct {
	ExpirationTime *metav1.Time `json:"expirationTime,omitempty"`
	Active         bool         `json:"active"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Duration",type=string,JSONPath=`.spec.ttl`,description="Duration for the challenge instance"
// +kubebuilder:printcolumn:name="Challenge REF",type=string,JSONPath=`.spec.challengeRef`,description="Challenge Reference"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ChallengeInstance represents an instance of a challenge
// +kubebuilder:resource:scope=Cluster,shortName={instance,instances}
type ChallengeInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ChallengeInstanceSpec   `json:"spec,omitempty"`
	Status ChallengeInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ChallengeInstanceList contains a list of ChallengeInstance.
type ChallengeInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ChallengeInstance `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ChallengeInstance{}, &ChallengeInstanceList{})
}
