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

package mutation

import (
	"context"
	"testing"
	"time"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = kubeflagv1.AddToScheme(scheme)
	return scheme
}

func durationPtr(d time.Duration) *metav1.Duration {
	return &metav1.Duration{Duration: d}
}

func challengeWithTTL(name string, ttl *metav1.Duration) *kubeflagv1.Challenge {
	return &kubeflagv1.Challenge{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       kubeflagv1.ChallengeSpec{DefaultTTL: ttl},
		Status:     kubeflagv1.ChallengeStatus{Healthy: true},
	}
}

func TestMutate_DefaultsTTLFromChallenge(t *testing.T) {
	scheme := newScheme()
	challenge := challengeWithTTL("web-challenge", durationPtr(15*time.Minute))
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(challenge).Build()

	instance := &kubeflagv1.ChallengeInstance{
		Spec: kubeflagv1.ChallengeInstanceSpec{
			ChallengeRef: "web-challenge",
			User:         "player1",
			// TTL is nil — should be defaulted
		},
	}

	m := NewMutator(client)
	mutated, err := m.Mutate(context.Background(), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated.Spec.TTL == nil {
		t.Fatal("expected TTL to be defaulted, got nil")
	}
	if mutated.Spec.TTL.Duration != 15*time.Minute {
		t.Errorf("expected TTL 15m, got %v", mutated.Spec.TTL.Duration)
	}
}

func TestMutate_PreservesExistingTTL(t *testing.T) {
	scheme := newScheme()
	challenge := challengeWithTTL("web-challenge", durationPtr(15*time.Minute))
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(challenge).Build()

	instance := &kubeflagv1.ChallengeInstance{
		Spec: kubeflagv1.ChallengeInstanceSpec{
			ChallengeRef: "web-challenge",
			User:         "player1",
			TTL:          durationPtr(30 * time.Minute),
		},
	}

	m := NewMutator(client)
	mutated, err := m.Mutate(context.Background(), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated.Spec.TTL.Duration != 30*time.Minute {
		t.Errorf("expected TTL to remain 30m, got %v", mutated.Spec.TTL.Duration)
	}
}

func TestMutate_InjectsChallengeRefLabel(t *testing.T) {
	scheme := newScheme()
	challenge := challengeWithTTL("web-challenge", durationPtr(15*time.Minute))
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(challenge).Build()

	instance := &kubeflagv1.ChallengeInstance{
		Spec: kubeflagv1.ChallengeInstanceSpec{
			ChallengeRef: "web-challenge",
			User:         "player1",
			TTL:          durationPtr(10 * time.Minute),
		},
	}

	m := NewMutator(client)
	mutated, err := m.Mutate(context.Background(), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	val, ok := mutated.Labels["challengeRef"]
	if !ok {
		t.Fatal("expected challengeRef label to be set")
	}
	if val != "web-challenge" {
		t.Errorf("expected label value %q, got %q", "web-challenge", val)
	}
}

func TestMutate_PreservesExistingLabels(t *testing.T) {
	scheme := newScheme()
	challenge := challengeWithTTL("web-challenge", durationPtr(15*time.Minute))
	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(challenge).Build()

	instance := &kubeflagv1.ChallengeInstance{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"team": "blue",
			},
		},
		Spec: kubeflagv1.ChallengeInstanceSpec{
			ChallengeRef: "web-challenge",
			User:         "player1",
			TTL:          durationPtr(10 * time.Minute),
		},
	}

	m := NewMutator(client)
	mutated, err := m.Mutate(context.Background(), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mutated.Labels["team"] != "blue" {
		t.Errorf("expected existing label 'team=blue' to be preserved, got %q", mutated.Labels["team"])
	}
	if mutated.Labels["challengeRef"] != "web-challenge" {
		t.Errorf("expected challengeRef label to be injected, got %q", mutated.Labels["challengeRef"])
	}
}

func TestMutate_ChallengeNotFound(t *testing.T) {
	scheme := newScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build() // no challenge in store

	instance := &kubeflagv1.ChallengeInstance{
		Spec: kubeflagv1.ChallengeInstanceSpec{
			ChallengeRef: "does-not-exist",
			User:         "player1",
		},
	}

	m := NewMutator(client)
	_, err := m.Mutate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error when Challenge does not exist, got nil")
	}
}

func TestMutate_SkipsWhenDeletionTimestampSet(t *testing.T) {
	scheme := newScheme()
	client := fake.NewClientBuilder().WithScheme(scheme).Build() // no challenge needed

	now := metav1.Now()
	instance := &kubeflagv1.ChallengeInstance{
		ObjectMeta: metav1.ObjectMeta{
			DeletionTimestamp: &now,
			Finalizers:        []string{"kubeflag.io/cleanup-instance"}, // required for non-zero DeletionTimestamp
		},
		Spec: kubeflagv1.ChallengeInstanceSpec{
			ChallengeRef: "web-challenge",
			User:         "player1",
		},
	}

	m := NewMutator(client)
	mutated, err := m.Mutate(context.Background(), instance)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return as-is — no TTL defaulted, no label set
	if mutated.Spec.TTL != nil {
		t.Errorf("expected TTL to remain nil for deleting instance, got %v", mutated.Spec.TTL)
	}
	if _, ok := mutated.Labels["challengeRef"]; ok {
		t.Error("expected no challengeRef label for deleting instance")
	}
}
