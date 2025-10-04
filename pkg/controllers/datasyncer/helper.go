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
		// Check if the label "DataSyncLabelKey" exists and its value is "true"
		if value, labelExists := labels[ManagedLabel]; labelExists && value == "true" {
			// Check if the label "datasyncer.kubeflag.io/source" exists
			if _, sourceLabelExists := labels[SourceLabel]; sourceLabelExists {
				return false // Not a source if both conditions are met
			}
		}
	}

	// If either condition is not met, the object is a source
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
