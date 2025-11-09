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

// AdmissionHandler for mutating KubeFlag Challenge CRD.
type AdmissionHandler struct {
	log      *logr.Logger
	decoder  admission.Decoder
	client   ctrlruntimeclient.Client
	caBundle *x509.CertPool
}

// NewAdmissionHanlder return a new challenge AdmissionHandler.
func NewAdmissionHanlder(log *logr.Logger, scheme *runtime.Scheme, client ctrlruntimeclient.Client, caBundle *x509.CertPool) *AdmissionHandler {
	return &AdmissionHandler{
		log:      log,
		decoder:  admission.NewDecoder(scheme),
		client:   client,
		caBundle: caBundle,
	}
}

func (h *AdmissionHandler) SetupWebhookWithManager(mgr ctrlruntime.Manager) {
	mgr.GetWebhookServer().Register("/mutate-challenge-v1", &webhook.Admission{Handler: h})
}

func (h *AdmissionHandler) Handle(ctx context.Context, req webhook.AdmissionRequest) webhook.AdmissionResponse {
	challenge := &kubeflagv1.Challenge{}
	var oldChallenge *kubeflagv1.Challenge

	switch req.Operation {
	case admissionv1.Create:
		if err := h.decoder.Decode(req, challenge); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

	case admissionv1.Update:
		if err := h.decoder.Decode(req, challenge); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

		oldChallenge = &kubeflagv1.Challenge{}
		if err := h.decoder.DecodeRaw(req.OldObject, oldChallenge); err != nil {
			return admission.Errored(http.StatusBadRequest, err)
		}

	case admissionv1.Delete:
		return webhook.Allowed(fmt.Sprintf("no mutation done for request %s", req.UID))

	default:
		return admission.Errored(http.StatusBadRequest, fmt.Errorf("%s not supported on challenge resources", req.Operation))
	}

	mutator := NewMutator(h.client, h.caBundle)

	mutated, mutateErr := mutator.Mutate(ctx, oldChallenge, challenge)
	if mutateErr != nil {
		h.log.Error(mutateErr, "challenge mutation failed")

		status := http.StatusBadRequest
		if mutateErr.Type == field.ErrorTypeInternal {
			status = http.StatusInternalServerError
		}

		return webhook.Errored(int32(status), mutateErr)
	}

	mutatedchallenge, err := json.Marshal(mutated)
	if err != nil {
		return webhook.Errored(http.StatusInternalServerError, fmt.Errorf("marshaling challenge object failed: %w", err))
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, mutatedchallenge)
}
