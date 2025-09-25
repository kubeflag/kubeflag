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
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Helper to create a Secret (implements metav1.Object)
func newSecretWithAnnotations(ann map[string]string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test",
			Namespace:   "default",
			Annotations: ann,
		},
	}
}

func TestAnnotateResourceWithChallenges(t *testing.T) {
	key := DataObjectAnnotationKey

	tests := []struct {
		name               string
		initialAnnotations map[string]string
		challengeName      string
		expectedChallenges []string
	}{
		{
			name:               "Add first challenge",
			initialAnnotations: nil,
			challengeName:      "challenge1",
			expectedChallenges: []string{"challenge1"},
		},
		{
			name:               "Add second challenge",
			initialAnnotations: map[string]string{key: `["challenge1"]`},
			challengeName:      "challenge2",
			expectedChallenges: []string{"challenge1", "challenge2"},
		},
		{
			name:               "Add duplicate challenge",
			initialAnnotations: map[string]string{key: `["challenge1"]`},
			challengeName:      "challenge1",
			expectedChallenges: []string{"challenge1"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			secretObject := newSecretWithAnnotations(test.initialAnnotations)
			AnnotateResourceWithChallenges(secretObject, test.challengeName)
			got := secretObject.GetAnnotations()[key]
			var gotChallenges []string
			_ = json.Unmarshal([]byte(got), &gotChallenges)
			if !reflect.DeepEqual(gotChallenges, test.expectedChallenges) {
				t.Errorf("unexpected challenges: got %v, want %v", gotChallenges, test.expectedChallenges)
			}
		})
	}
}

func TestRemoveChallengeFromResource(t *testing.T) {
	key := DataObjectAnnotationKey

	tests := []struct {
		name               string
		initialAnnotations map[string]string
		removeName         string
		expectedChallenges []string // empty slice ⇒ annotation should be deleted
	}{

		{
			name:               "Remove from empty annotations",
			initialAnnotations: nil,
			removeName:         "challenge1",
			expectedChallenges: nil, // expect key to be gone
		},
		{
			name:               "Remove existing single challenge",
			initialAnnotations: map[string]string{key: `["challenge1"]`},
			removeName:         "challenge1",
			expectedChallenges: nil, // expect key to be gone
		},
		{
			name:               "Remove one of multiple",
			initialAnnotations: map[string]string{key: `["challenge1","challenge2"]`},
			removeName:         "challenge1",
			expectedChallenges: []string{"challenge2"},
		},
		{
			name:               "Remove non-existent challenge",
			initialAnnotations: map[string]string{key: `["challenge1"]`},
			removeName:         "does-not-exist",
			expectedChallenges: []string{"challenge1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := newSecretWithAnnotations(tt.initialAnnotations)

			// Call the function under test
			RemoveChallengeFromResource(secret, tt.removeName)

			anns := secret.GetAnnotations()
			got, exists := anns[key]

			if len(tt.expectedChallenges) == 0 {
				// When we expect an empty list, the key itself should be removed
				if exists {
					t.Errorf("expected annotation key %q to be deleted, but found: %v", key, got)
				}
				return
			}

			if !exists {
				t.Fatalf("expected annotation key %q to exist", key)
			}

			var gotChallenges []string
			if err := json.Unmarshal([]byte(got), &gotChallenges); err != nil {
				t.Fatalf("failed to unmarshal annotations: %v", err)
			}

			if !reflect.DeepEqual(gotChallenges, tt.expectedChallenges) {
				t.Errorf("unexpected challenges: got %v, want %v", gotChallenges, tt.expectedChallenges)
			}
		})
	}
}

func TestHasChallengesAnnotation(t *testing.T) {
	key := DataObjectAnnotationKey
	sec := newSecretWithAnnotations(map[string]string{key: `["foo"]`})
	if !HasChallengesAnnotation(sec) {
		t.Errorf("expected HasChallengesAnnotation to be true")
	}

	sec = newSecretWithAnnotations(nil)
	if HasChallengesAnnotation(sec) {
		t.Errorf("expected false for nil annotations")
	}
}

func TestHashTemplate(t *testing.T) {
	spec := corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "c",
			Image: "busybox",
		}},
	}
	got, err := hashTemplate(spec)
	if err != nil {
		t.Fatalf("hashTemplate returned error: %v", err)
	}

	// Re-hash to compare deterministically
	data, _ := json.Marshal(spec)
	want := sha256.Sum256(data)
	if got != hex.EncodeToString(want[:]) {
		t.Errorf("hash mismatch: got %s, want %s", got, hex.EncodeToString(want[:]))
	}
}
