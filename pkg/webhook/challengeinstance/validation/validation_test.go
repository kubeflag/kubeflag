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
	"time"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrlruntimefakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newScheme returns a scheme with kubeflagv1 types registered.
func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = kubeflagv1.AddToScheme(scheme)
	return scheme
}

// healthyChallenge returns a Challenge with status.healthy=true.
func healthyChallenge(name string) *kubeflagv1.Challenge {
	return &kubeflagv1.Challenge{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     kubeflagv1.ChallengeStatus{Healthy: true},
	}
}

// unhealthyChallenge returns a Challenge with status.healthy=false.
func unhealthyChallenge(name string) *kubeflagv1.Challenge {
	return &kubeflagv1.Challenge{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     kubeflagv1.ChallengeStatus{Healthy: false},
	}
}

// durationPtr returns a pointer to a metav1.Duration.
func durationPtr(d time.Duration) *metav1.Duration {
	return &metav1.Duration{Duration: d}
}

func TestValidateCreate(t *testing.T) {
	tests := []struct {
		name      string
		instance  *kubeflagv1.ChallengeInstance
		objects   []runtime.Object
		expectErr bool
	}{
		{
			name: "valid instance with TTL",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
					TTL:          durationPtr(10 * time.Minute),
				},
			},
			objects:   []runtime.Object{healthyChallenge("web-challenge")},
			expectErr: false,
		},
		{
			name: "valid instance without TTL (nil)",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
				},
			},
			objects:   []runtime.Object{healthyChallenge("web-challenge")},
			expectErr: false,
		},
		{
			name: "empty challengeRef",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "",
					User:         "player1",
				},
			},
			objects:   []runtime.Object{},
			expectErr: true,
		},
		{
			name: "challengeRef references non-existent Challenge",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "does-not-exist",
					User:         "player1",
				},
			},
			objects:   []runtime.Object{},
			expectErr: true,
		},
		{
			name: "referenced Challenge is not healthy",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "broken-challenge",
					User:         "player1",
				},
			},
			objects:   []runtime.Object{unhealthyChallenge("broken-challenge")},
			expectErr: true,
		},
		{
			name: "empty user",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "",
				},
			},
			objects:   []runtime.Object{healthyChallenge("web-challenge")},
			expectErr: true,
		},
		{
			name: "invalid user (not DNS-compatible)",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "INVALID_USER!",
				},
			},
			objects:   []runtime.Object{healthyChallenge("web-challenge")},
			expectErr: true,
		},
		{
			name: "negative TTL",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
					TTL:          durationPtr(-5 * time.Minute),
				},
			},
			objects:   []runtime.Object{healthyChallenge("web-challenge")},
			expectErr: true,
		},
		{
			name: "zero TTL",
			instance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
					TTL:          durationPtr(0),
				},
			},
			objects:   []runtime.Object{healthyChallenge("web-challenge")},
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
			_, err := v.ValidateCreate(context.Background(), tt.instance)

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
		name        string
		oldInstance *kubeflagv1.ChallengeInstance
		newInstance *kubeflagv1.ChallengeInstance
		expectErr   bool
	}{
		{
			name: "no changes",
			oldInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
				},
			},
			newInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
				},
			},
			expectErr: false,
		},
		{
			name: "changed challengeRef",
			oldInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
				},
			},
			newInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "another-challenge",
					User:         "player1",
				},
			},
			expectErr: true,
		},
		{
			name: "changed user",
			oldInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
				},
			},
			newInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player2",
				},
			},
			expectErr: true,
		},
		{
			name: "TTL change is allowed",
			oldInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
					TTL:          durationPtr(10 * time.Minute),
				},
			},
			newInstance: &kubeflagv1.ChallengeInstance{
				Spec: kubeflagv1.ChallengeInstanceSpec{
					ChallengeRef: "web-challenge",
					User:         "player1",
					TTL:          durationPtr(30 * time.Minute),
				},
			},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newScheme()
			client := ctrlruntimefakeclient.NewClientBuilder().WithScheme(scheme).Build()

			v := NewValidator(client)
			_, err := v.ValidateUpdate(context.Background(), tt.oldInstance, tt.newInstance)

			if tt.expectErr && err == nil {
				t.Errorf("expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}
