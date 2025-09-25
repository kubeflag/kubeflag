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

package challenge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/kubeflag/kubeflag/pkg/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DataObjectAnnotationKey = "data.kubeflag.io/challenges"

func RemoveChallengeFromResource(o metav1.Object, challengeName string) {
	// Parse the current annotation value
	annotations := o.GetAnnotations()
	var challengeList []string
	if existing, ok := annotations[DataObjectAnnotationKey]; ok && existing != "" {
		_ = json.Unmarshal([]byte(existing), &challengeList)
	}

	// Remove the challengeName
	updatedList := []string{}
	for _, name := range challengeList {
		if name != challengeName {
			updatedList = append(updatedList, name)
		}
	}

	// Update the annotation or delete it if the list is empty
	if len(updatedList) == 0 {
		delete(annotations, DataObjectAnnotationKey)
		kubernetes.EnsureAnnotations(o, annotations)
	} else {
		updated, _ := json.Marshal(updatedList)
		kubernetes.EnsureAnnotations(o, map[string]string{
			DataObjectAnnotationKey: string(updated),
		})
	}
}

func AnnotateResourceWithChallenges(o metav1.Object, challengeName string) {
	// Parse the current annotation value
	annotations := o.GetAnnotations()
	var challengeList []string
	if existing, ok := annotations[DataObjectAnnotationKey]; ok && existing != "" {
		_ = json.Unmarshal([]byte(existing), &challengeList)
	}

	// Add the challengeName if it doesn't exist
	if !contains(challengeList, challengeName) {
		challengeList = append(challengeList, challengeName)
	}

	// Marshal the updated list and ensure the annotation
	updated, _ := json.Marshal(challengeList)
	kubernetes.EnsureAnnotations(o, map[string]string{
		DataObjectAnnotationKey: string(updated),
	})
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Helper function to check for the annotation
func HasChallengesAnnotation(obj client.Object) bool {
	annotations := obj.GetAnnotations()
	_, exists := annotations[DataObjectAnnotationKey]
	return exists
}

func hashTemplate(spec corev1.PodSpec) (string, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}
