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

package mutation

import (
	"context"
	"crypto/x509"
	"time"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Mutator for mutating KubeFlag Challenge CRD.
type Mutator struct {
	client   ctrlruntimeclient.Client
	caBundle *x509.CertPool
}

// NewMutator returns a new challenge Mutator.
func NewMutator(client ctrlruntimeclient.Client, caBundle *x509.CertPool) *Mutator {
	return &Mutator{
		client:   client,
		caBundle: caBundle,
	}
}

func (m *Mutator) Mutate(ctx context.Context, oldChallenge, newChallenge *kubeflagv1.Challenge) (*kubeflagv1.Challenge, *field.Error) {
	// do not perform mutations on challenges in deletion
	if newChallenge.DeletionTimestamp != nil {
		return newChallenge, nil
	}

	DefaultChallengeSpec(ctx, &newChallenge.Spec)

	return newChallenge, nil
}

func DefaultChallengeSpec(ctx context.Context, spec *kubeflagv1.ChallengeSpec) {
	for _, secRef := range spec.SecretReferences {
		secRef.Kind = "secret"
	}

	for _, configRef := range spec.ConfigMapReferences {
		configRef.Kind = "configmap"
	}

	if spec.DefaultTTL == nil {
		spec.DefaultTTL = &metav1.Duration{Duration: 15 * time.Minute}
	}

	DefaultChallengeTemplate(&spec.Template)
}

func DefaultChallengeTemplate(template *kubeflagv1.DeploymentTemplate) {
	spec := template.Spec
	if spec.DNSPolicy == "" {
		spec.DNSPolicy = corev1.DNSClusterFirst
	}
	if spec.RestartPolicy == "" {
		spec.RestartPolicy = corev1.RestartPolicyAlways
	}
	if spec.SecurityContext == nil {
		spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	if spec.TerminationGracePeriodSeconds == nil {
		period := int64(corev1.DefaultTerminationGracePeriodSeconds)
		spec.TerminationGracePeriodSeconds = &period
	}
	if spec.SchedulerName == "" {
		spec.SchedulerName = corev1.DefaultSchedulerName
	}
	template.Spec = spec
}
