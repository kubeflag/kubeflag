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
	"fmt"
	"time"

	"github.com/go-logr/logr"

	"github.com/kubeflag/kubeflag/pkg/controllers/challenge"
	"github.com/kubeflag/kubeflag/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
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

var rawLog logr.Logger

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
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}

	// Predicate: has Challenge annotation
	hasChallengeAnno := predicate.NewPredicateFuncs(challenge.HasChallengesAnnotation)

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
	rawLog = log.WithName(ControllerName)
	return err
}

func (r *DataSyncerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	// Try Secret first
	secret := &corev1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err == nil {
		r.log = rawLog.WithValues("type", "secret", "name", secret.Name, "namespace", secret.Namespace)
		r.log.Info("Reconciling object")
		return r.reconcileDataObject(ctx, SecretWrapper{secret})
	} else if !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, configMap); err == nil {
		r.log = rawLog.WithValues("type", "configmap", "name", configMap.Name, "namespace", configMap.Namespace)
		r.log.Info("Reconciling object")
		return r.reconcileDataObject(ctx, ConfigMapWrapper{configMap})
	} else if !apierrors.IsNotFound(err) {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *DataSyncerReconciler) createOrUpdate(ctx context.Context, obj ctrlruntimeclient.Object) error {
	r.log.Info("Syncing the data object", "target", obj.GetNamespace())
	err := r.Create(ctx, obj)
	if err != nil && apierrors.IsAlreadyExists(err) {
		r.log.V(1).Info("The object is existing, updating...", "target", obj.GetNamespace())
		if err1 := r.Update(ctx, obj); err1 != nil {
			return err1
		}
		return nil
	}
	return err
}

func (r *DataSyncerReconciler) reconcileDataObject(ctx context.Context, obj SyncableObject) (reconcile.Result, error) {
	if isSource(obj) {
		challengesNames, err := getChallengeNamesFromAnnotation(obj)
		if err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to parse annotations: %w", err)
		}

		if obj.GetDeletionTimestamp() != nil && kubernetes.HasFinalizer(obj, CleanupFinalizer) {
			r.log.Info("Deleting the source")
			if len(challengesNames) > 0 {
				if err = r.cleanupCopies(ctx, obj, challengesNames); err != nil {
					return reconcile.Result{}, err
				}
			}

			kubernetes.RemoveFinalizer(obj, CleanupFinalizer)
			if err = r.Update(ctx, obj.GetBaseObject()); err != nil {
				return reconcile.Result{}, err
			}
			return reconcile.Result{}, nil
		}

		if err = r.cleanupUndesiredCopies(ctx, obj, challengesNames); err != nil {
			return reconcile.Result{}, err
		}

		for _, challengeName := range challengesNames {
			if err = r.syncToNamespace(ctx, obj, challengeName); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to sync %s to namespace %s: %w", obj.GetTypeMeta().Kind, challengeName, err)
			}
			if !kubernetes.HasFinalizer(obj, CleanupFinalizer) {
				kubernetes.AddFinalizer(obj, CleanupFinalizer)
				if err = r.Update(ctx, obj.GetBaseObject()); err != nil {
					return reconcile.Result{}, err
				}
			}
		}
	} else {
		var source ctrlruntimeclient.Object
		switch obj.(type) {
		case SecretWrapper:
			source = &corev1.Secret{}
		case ConfigMapWrapper:
			source = &corev1.ConfigMap{}
		}

		namespaced := getSource(obj)
		if namespaced == nil {
			return reconcile.Result{}, fmt.Errorf("object don't have any source")
		}

		if err := r.Get(ctx, *namespaced, source); err != nil {
			return reconcile.Result{}, err
		}

		// If object is deleted now, reconcile the source to create another one if it's necessary.
		if obj.GetDeletionTimestamp() != nil && kubernetes.HasAnyFinalizer(obj, SyncFinalizer) {
			r.log.Info("Deleting the copie...")
			if source.GetDeletionTimestamp() == nil {
				r.log.Info("The source is still existing, reconciling it to create another copy...")
				annotations := source.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations["datasyncer.kubeflag.io/last-trigger"] = time.Now().Format(time.RFC3339)

				source.SetAnnotations(annotations)
				if err := r.Update(ctx, source); err != nil {
					return reconcile.Result{}, err
				}
			}
			kubernetes.RemoveFinalizer(obj, SyncFinalizer)
			if err := r.Update(ctx, obj.GetBaseObject()); err != nil {
				return reconcile.Result{}, fmt.Errorf("failed to remove finalizer in object %w", err)
			}
			return reconcile.Result{}, nil
		}
		var syncable SyncableObject
		switch s := source.(type) {
		case *corev1.Secret:
			syncable = &SecretWrapper{Secret: s}
		case *corev1.ConfigMap:
			syncable = &ConfigMapWrapper{ConfigMap: s}
		default:
			return reconcile.Result{}, fmt.Errorf("unsupported source type: %T", source)
		}
		if !equality.Semantic.DeepEqual(syncable.GetData(), obj.GetData()) {
			r.log.V(1).Info("Copied object is not synced to the source, Syncing...")
			obj.SetData(syncable.GetData())
			return reconcile.Result{}, r.Update(ctx, obj.GetBaseObject())
		}
	}

	return reconcile.Result{}, nil
}

func (r *DataSyncerReconciler) syncToNamespace(ctx context.Context, source SyncableObject, targetNamespace string) error {
	var newObj ctrlruntimeclient.Object
	switch s := source.(type) {
	case SecretWrapper:
		newObj = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.GetName(),
				Namespace: targetNamespace,
				Labels: map[string]string{
					ManagedLabel: "true",
					SourceLabel:  fmt.Sprintf("%s---%s", s.GetNamespace(), s.GetName()),
				},
				Finalizers: []string{SyncFinalizer},
			},
			Data: source.GetData(),
		}
		return r.createOrUpdate(ctx, newObj)

	case ConfigMapWrapper:
		newObj = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      s.GetName(),
				Namespace: targetNamespace,
				Labels: map[string]string{
					ManagedLabel: "true",
					SourceLabel:  fmt.Sprintf("%s---%s", s.GetNamespace(), s.GetName()),
				},
				Finalizers: []string{SyncFinalizer},
			},
			BinaryData: s.GetData(),
		}
	}
	return r.createOrUpdate(ctx, newObj)
}

func (r *DataSyncerReconciler) cleanupCopies(ctx context.Context, source SyncableObject, challenges []string) error {
	// Build a set for quick membership tests
	desired := sets.NewString(challenges...)
	if len(desired) > 0 {
		r.log.Info("CleaningUp the copies...")
		var copie ctrlruntimeclient.Object
		for _, ns := range desired.List() {
			switch source.(type) {
			case *SecretWrapper:
				copie = &corev1.Secret{}
			case *ConfigMapWrapper:
				copie = &corev1.ConfigMap{}
			}
			key := types.NamespacedName{Name: source.GetGenerateName(), Namespace: ns}

			err := r.Get(ctx, key, copie)
			if apierrors.IsNotFound(err) {
				// nothing to delete in this namespace
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to get %s %s/%s: %w", source.GetTypeMeta().Kind, ns, source.GetName(), err)
			}

			// Optional safety check: ensure it is a managed copy
			if copie.GetLabels() != nil && copie.GetLabels()[ManagedLabel] != "true" {
				continue
			}

			if err := r.Delete(ctx, copie); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to delete %s %s/%s: %w", source.GetTypeMeta().Kind, ns, source.GetName(), err)
			}
		}
	} else {
		r.log.Info("There is no copies to delete")
	}
	return nil
}

func (r *DataSyncerReconciler) cleanupUndesiredCopies(ctx context.Context, source SyncableObject, desiredNamespaces []string) error {
	r.log.Info("Looking for unwanted copies")
	desired := sets.NewString(desiredNamespaces...)

	selector := ctrlruntimeclient.MatchingLabels{
		ManagedLabel: "true",
		SourceLabel:  fmt.Sprintf("%s---%s", source.GetNamespace(), source.GetName()),
	}

	var list ctrlruntimeclient.ObjectList
	switch source.(type) {
	case SecretWrapper:
		list = &corev1.SecretList{}
	case ConfigMapWrapper:
		list = &corev1.ConfigMapList{}
	}

	if err := r.List(ctx, list, selector); err != nil {
		return fmt.Errorf("list managed copies: %w", err)
	}

	items, err := meta.ExtractList(list)
	if err != nil {
		return fmt.Errorf("extract list items: %w", err)
	}
	if len(items) > 0 {
		r.log.V(1).Info("Deleting undesired copies...")
		for _, s := range items {
			// ✨ Convert to client.Object (has GetName()/GetNamespace())
			obj, ok := s.(ctrlruntimeclient.Object)
			if !ok {
				return fmt.Errorf("failt to convert runtime.Object to ctrlruntimeclient.Object")
			}
			if desired.Has(obj.GetNamespace()) {
				continue // still wanted
			}

			// Delete copies in namespaces no longer desired
			if err := r.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) {
				return fmt.Errorf("delete stale copy %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
		}
	} else {
		r.log.V(1).Info("No undesired copies to delete")
	}
	return nil
}
