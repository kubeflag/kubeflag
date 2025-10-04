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
	"context"

	"github.com/go-logr/logr"

	"github.com/kubeflag/kubeflag/pkg/controllers/challenge"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type DataSyncerReconciler struct {
	ctrlruntimeclient.Client
	log      logr.Logger
	recorder record.EventRecorder
}

const (
	ControllerName   = "datasyncer-controller"
	ManagedLabel     = "datasyncer.kubeflag.io/managed"
	SourceLabel      = "datasyncer.kubeflag.io/source"
	CleanupFinalizer = "datasyncer.kubeflag.io/cleanup-synced-objects"
	SyncFinalizer    = "datasyncer.kubeflag.io/synced"
)

// Add creates a new Challenge controller and adds it to the Manager.
func Add(ctx context.Context, mgr manager.Manager, numWorkers int, log *logr.Logger) error {
	reconciler := &DataSyncerReconciler{
		Client:   mgr.GetClient(),
		log:      log.WithName(ControllerName),
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}

	// Predicate: has Challenge annotation
	hasChallengeAnno := predicate.NewPredicateFuncs(func(obj ctrlruntimeclient.Object) bool {
		return challenge.HasChallengesAnnotation(obj)
	})

	// Predicate: has managed label
	hasManagedLabel := predicate.NewPredicateFuncs(func(obj ctrlruntimeclient.Object) bool {
		return obj.GetLabels()[ManagedLabel] == "true"
	})

	// OR: either annotated or managed
	sourceOrCopyPredicate := predicate.Or(hasChallengeAnno, hasManagedLabel)

	// Build controller: watch Secrets and ConfigMaps matching either predicate
	_, err := builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers}).
		For(&corev1.Secret{}, builder.WithPredicates(sourceOrCopyPredicate)).
		Watches(
			&corev1.ConfigMap{},
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(sourceOrCopyPredicate),
		).
		Build(reconciler)

	return err
}

func (r *DataSyncerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log.WithValues("resource", req.NamespacedName)
	// Try Secret first
	secret := &corev1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err == nil {
		r.log.Info("Reconciling Secret", "name", secret.Name)
		return r.reconcileDataObject(ctx, SecretWrapper{secret})
	} else if !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
}

func (r *DataSyncerReconciler) createOrUpdate(ctx context.Context, obj ctrlruntimeclient.Object) error {
	r.log.Info("Syncing the data object", "object", obj.GetObjectKind())
	err := r.Create(ctx, obj)
	if err != nil && apierrors.IsAlreadyExists(err) {
		return r.Update(ctx, obj)
	}
	return err
}
