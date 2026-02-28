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
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-logr/logr"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrlruntime "sigs.k8s.io/controller-runtime"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// AdmissionHandler for mutating KubeFlag ChallengeInstance CRD.
type AdmissionHandler struct {
	log     *logr.Logger
	decoder admission.Decoder
	client  ctrlruntimeclient.Client
}

// NewAdmissionHandler returns a new ChallengeInstance AdmissionHandler.
func NewAdmissionHandler(log *logr.Logger, scheme *runtime.Scheme, client ctrlruntimeclient.Client) *AdmissionHandler {
	return &AdmissionHandler{
		log:     log,
		decoder: admission.NewDecoder(scheme),
		client:  client,
	}
}

func (h *AdmissionHandler) SetupWebhookWithManager(mgr ctrlruntime.Manager) {
	mgr.GetWebhookServer().Register("/mutate-challengeinstance-v1", &webhook.Admission{Handler: h})
}

func (h *AdmissionHandler) Handle(ctx context.Context, req webhook.AdmissionRequest) webhook.AdmissionResponse {
	instance := &kubeflagv1.ChallengeInstance{}

	switch req.Operation {
	case admissionv1.Create:
		if err := h.decoder.Decode(req, instance); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

	case admissionv1.Update, admissionv1.Delete:
		return webhook.Allowed(fmt.Sprintf("no mutation done for request %s", req.UID))

	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s not supported on challengeinstance resources", req.Operation))
	}

	mutator := NewMutator(h.client)

	mutated, mutateErr := mutator.Mutate(ctx, instance)
	if mutateErr != nil {
		h.log.Error(mutateErr, "challengeinstance mutation failed")

		status := http.StatusBadRequest
		if mutateErr.Type == field.ErrorTypeInternal {
			status = http.StatusInternalServerError
		}

		return webhook.Errored(int32(status), mutateErr)
	}

	mutatedInstance, err := json.Marshal(mutated)
	if err != nil {
		return webhook.Errored(http.StatusInternalServerError, fmt.Errorf("marshaling challengeinstance object failed: %w", err))
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, mutatedInstance)
}
