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

package datasyncer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kubeflag/kubeflag/pkg/controllers/challenge"

	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Helper to parse challenge names from annotation.
func getChallengeNamesFromAnnotation(obj ctrlruntimeclient.Object) ([]string, error) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil, fmt.Errorf("no annotations found")
	}
	annotationValue, exists := annotations[challenge.DataObjectAnnotationKey]
	if !exists {
		return nil, fmt.Errorf("no challenges annotation found")
	}
	var challengeNames []string
	if err := json.Unmarshal([]byte(annotationValue), &challengeNames); err != nil {
		return nil, fmt.Errorf("failed to parse challenges annotation: %w", err)
	}
	return challengeNames, nil
}

func isSource(object ctrlruntimeclient.Object) bool {
	// Retrieve labels from the object
	labels := object.GetLabels()

	// Check if both conditions are satisfied to determine if it's not a source
	if labels != nil {
		// Check if the label "ManagedLabel" exists and its value is "true"
		if value, labelExists := labels[ManagedLabel]; labelExists && value == "true" {
			// Check if the label "datasyncer.kubeflag.io/source" exists
			if _, sourceLabelExists := labels[SourceLabel]; sourceLabelExists {
				return false
			}
		}
	}

	return true
}

func getSource(object ctrlruntimeclient.Object) *types.NamespacedName {
	labels := object.GetLabels()
	if labels != nil {
		if value, labelExists := labels[SourceLabel]; labelExists {
			name := strings.Split(value, "---")[1]
			namespace := strings.Split(value, "---")[0]
			return &types.NamespacedName{
				Name:      name,
				Namespace: namespace,
			}
		}
	}
	return nil
}
