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
	"fmt"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Mutator for mutating KubeFlag ChallengeInstance CRD.
type Mutator struct {
	client ctrlruntimeclient.Client
}

// NewMutator returns a new ChallengeInstance Mutator.
func NewMutator(client ctrlruntimeclient.Client) *Mutator {
	return &Mutator{
		client: client,
	}
}

func (m *Mutator) Mutate(ctx context.Context, instance *kubeflagv1.ChallengeInstance) (*kubeflagv1.ChallengeInstance, *field.Error) {
	// Do not perform mutations on instances in deletion.
	if instance.DeletionTimestamp != nil {
		return instance, nil
	}

	// Look up the referenced Challenge to get defaults.
	challenge := &kubeflagv1.Challenge{}
	if err := m.client.Get(ctx, types.NamespacedName{Name: instance.Spec.ChallengeRef}, challenge); err != nil {
		return nil, field.InternalError(
			field.NewPath("spec", "challengeRef"),
			fmt.Errorf("failed to look up Challenge %q: %w", instance.Spec.ChallengeRef, err),
		)
	}

	// Default TTL from the Challenge's DefaultTTL if not set.
	if instance.Spec.TTL == nil && challenge.Spec.DefaultTTL != nil {
		defaultTTL := *challenge.Spec.DefaultTTL
		instance.Spec.TTL = &defaultTTL
	}

	// Set the challengeRef label.
	if instance.Labels == nil {
		instance.Labels = make(map[string]string)
	}
	instance.Labels["challengeRef"] = instance.Spec.ChallengeRef

	return instance, nil
}
