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

package consumer

import (
	"context"
	"crypto/rsa"
	"time"

	"github.com/go-logr/logr"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	"github.com/kubeflag/kubeflag/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	ControllerName   = "consumer-controller"
	CleanupFinalizer = "kubeflag.io/cleanup-consumer"

	tokenSecretNamespace = signingKeySecretNamespace

	// TokenSecretLabel is applied to every token Secret so the controller can
	// watch them and re-issue the token when one is deleted.
	TokenSecretLabel = "kubeflag.io/consumer"
)

var rawLog *logr.Logger

type ConsumerReconciler struct {
	ctrlruntimeclient.Client
	log        logr.Logger
	signingKey *rsa.PrivateKey
	signingKID string
}

// Add creates a new Consumer controller and registers it with the Manager.
func Add(_ context.Context, mgr manager.Manager, numWorkers int, log *logr.Logger) error {
	reconciler := &ConsumerReconciler{
		Client: mgr.GetClient(),
	}

	rawLog = log

	_, err := builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers}).
		For(&kubeflagv1.Consumer{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Watch token Secrets so that a manual deletion triggers re-issuance.
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj ctrlruntimeclient.Object) []reconcile.Request {
				consumerName, ok := obj.GetLabels()[TokenSecretLabel]
				if !ok {
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: consumerName}}}
			}),
			builder.WithPredicates(predicate.NewPredicateFuncs(func(obj ctrlruntimeclient.Object) bool {
				_, ok := obj.GetLabels()[TokenSecretLabel]
				return ok
			})),
		).
		Build(reconciler)

	return err
}

func (r *ConsumerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log = rawLog.WithName(ControllerName).WithValues("Consumer", req.Name)

	// Lazy-init: load or generate the signing key on the first reconcile.
	// Retried on every call until it succeeds.
	if r.signingKey == nil {
		key, kid, err := EnsureSigningKey(ctx, r.Client)
		if err != nil {
			r.log.Error(err, "Failed to ensure signing key")
			return reconcile.Result{}, err
		}
		r.signingKey = key
		r.signingKID = kid
		r.log.Info("Signing key loaded", "kid", r.signingKID)
	}

	consumer := &kubeflagv1.Consumer{}
	if err := r.Get(ctx, req.NamespacedName, consumer); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Ensure finalizer is present before doing anything.
	if consumer.GetDeletionTimestamp() == nil && !kubernetes.HasFinalizer(consumer, CleanupFinalizer) {
		kubernetes.AddFinalizer(consumer, CleanupFinalizer)
		if err := r.Update(ctx, consumer); err != nil {
			r.log.Error(err, "Failed to add finalizer")
			return reconcile.Result{}, err
		}
	}

	// Deletion: clean up the token Secret, then remove the finalizer.
	if consumer.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, r.reconcileDelete(ctx, consumer)
	}

	// Compute the phase the controller wants the Consumer to be in.
	desiredPhase := computePhase(consumer)

	// Sync phase into status when it has changed.
	if consumer.Status.Phase != desiredPhase {
		r.log.Info("Phase changed", "from", consumer.Status.Phase, "to", desiredPhase)
		consumer.Status.Phase = desiredPhase
		if err := r.Status().Update(ctx, consumer); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Only Active consumers get a token.
	if desiredPhase != kubeflagv1.ConsumerPhaseActive {
		return reconcile.Result{}, nil
	}

	return r.reconcileActive(ctx, consumer)
}

// reconcileActive handles token issuance (and re-issuance) for Active consumers.
func (r *ConsumerReconciler) reconcileActive(ctx context.Context, consumer *kubeflagv1.Consumer) (reconcile.Result, error) {
	// If a token Secret is already referenced, check it still exists.
	if consumer.Status.TokenSecretRef != nil {
		existing := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      consumer.Status.TokenSecretRef.Name,
			Namespace: consumer.Status.TokenSecretRef.Namespace,
		}, existing)
		if err == nil {
			// Token is present — nothing to do.
			return reconcile.Result{}, nil
		}
		if !apierrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
		// Secret was deleted — clear the ref and fall through to reissue.
		r.log.Info("Token Secret was deleted, reissuing")
		consumer.Status.TokenSecretRef = nil
		consumer.Status.IssuedAt = nil
		if err := r.Status().Update(ctx, consumer); err != nil {
			return reconcile.Result{}, err
		}
	}

	// Validate that the referenced Tenant exists before issuing.
	tenant := &kubeflagv1.Tenant{}
	if err := r.Get(ctx, types.NamespacedName{Name: consumer.Spec.TenantRef}, tenant); err != nil {
		if apierrors.IsNotFound(err) {
			r.log.Info("TenantRef not found, will retry", "tenantRef", consumer.Spec.TenantRef)
			return reconcile.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return reconcile.Result{}, err
	}

	// Issue the JWT.
	tokenStr, err := IssueToken(r.signingKey, r.signingKID, consumer)
	if err != nil {
		r.log.Error(err, "Failed to issue JWT")
		return reconcile.Result{}, err
	}

	// Persist the token in a Secret inside kubeflag-system.
	secretName := consumer.Name + "-token"
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: tokenSecretNamespace,
			Labels: map[string]string{
				TokenSecretLabel: consumer.Name,
			},
		},
		Data: map[string][]byte{
			"token": []byte(tokenStr),
		},
	}
	if err := r.Create(ctx, tokenSecret); err != nil && !apierrors.IsAlreadyExists(err) {
		r.log.Error(err, "Failed to create token Secret")
		return reconcile.Result{}, err
	}

	// Update status to record where the token lives.
	now := metav1.Now()
	consumer.Status.TokenSecretRef = &corev1.SecretReference{
		Name:      secretName,
		Namespace: tokenSecretNamespace,
	}
	consumer.Status.IssuedAt = &now
	if err := r.Status().Update(ctx, consumer); err != nil {
		return reconcile.Result{}, err
	}

	r.log.Info("JWT issued", "secret", secretName)
	return reconcile.Result{}, nil
}

// reconcileDelete cleans up the token Secret and removes the finalizer.
func (r *ConsumerReconciler) reconcileDelete(ctx context.Context, consumer *kubeflagv1.Consumer) error {
	if !kubernetes.HasFinalizer(consumer, CleanupFinalizer) {
		return nil
	}

	if consumer.Status.TokenSecretRef != nil {
		tokenSecret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      consumer.Status.TokenSecretRef.Name,
			Namespace: consumer.Status.TokenSecretRef.Namespace,
		}, tokenSecret)
		if err == nil {
			if delErr := r.Delete(ctx, tokenSecret); delErr != nil && !apierrors.IsNotFound(delErr) {
				r.log.Error(delErr, "Failed to delete token Secret")
				return delErr
			}
		} else if !apierrors.IsNotFound(err) {
			return err
		}
	}

	kubernetes.RemoveFinalizer(consumer, CleanupFinalizer)
	if err := r.Update(ctx, consumer); err != nil {
		r.log.Error(err, "Failed to remove finalizer")
		return err
	}

	r.log.Info("Consumer deleted, token Secret cleaned up")
	return nil
}

// computePhase derives the desired ConsumerPhase from the spec.
func computePhase(consumer *kubeflagv1.Consumer) kubeflagv1.ConsumerPhase {
	if consumer.Spec.Suspended {
		return kubeflagv1.ConsumerPhaseSuspended
	}
	if consumer.Spec.ExpiresAt != nil && consumer.Spec.ExpiresAt.Time.Before(time.Now()) {
		return kubeflagv1.ConsumerPhaseExpired
	}
	return kubeflagv1.ConsumerPhaseActive
}
