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
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/kubeflag/kubeflag/pkg/controllers/challenge"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
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

const ControllerName = "datasyncer-controller"

// Add creates a new Challenge controller and adds it to the Manager.
func Add(ctx context.Context, mgr manager.Manager, numWorkers int, log *logr.Logger) error {
	reconciler := &DataSyncerReconciler{
		Client:   mgr.GetClient(),
		log:      log.WithName(ControllerName),
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}

	// Define a predicate to filter resources with the annotation
	annotatedResourcesPredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return challenge.HasChallengesAnnotation(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return challenge.HasChallengesAnnotation(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return challenge.HasChallengesAnnotation(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return challenge.HasChallengesAnnotation(e.Object)
		},
	}

	// Set up the controller with the reconciler
	_, err := builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: numWorkers,
		}).
		For(&corev1.Secret{}).
		WithEventFilter(annotatedResourcesPredicate). // Add predicate to filter events
		Watches(
			&corev1.ConfigMap{}, // Watch ConfigMaps
			&handler.EnqueueRequestForObject{},
			builder.WithPredicates(annotatedResourcesPredicate), // Add predicate for ConfigMaps
		).
		Build(reconciler)

	return err
}

func (r *DataSyncerReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log.WithValues("resource", req.NamespacedName)
	// Attempt to fetch the resource as a Secret
	secret := &corev1.Secret{}
	if err := r.Get(ctx, req.NamespacedName, secret); err == nil {
		r.log.Info("Reconciling Secret", "name", secret.Name, "namespace", secret.Namespace)
		return r.reconcileSecret(ctx, secret)
	}

	// If it's not a Secret, attempt to fetch it as a ConfigMap
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, req.NamespacedName, configMap); err == nil {
		r.log.Info("Reconciling ConfigMap", "name", configMap.Name, "namespace", configMap.Namespace)
		return r.reconcileConfigMap(ctx, configMap)
	}

	// If neither, log an error
	r.log.Error(fmt.Errorf("resource is neither Secret nor ConfigMap"), "Invalid resource type", "name", req.Name, "namespace", req.Namespace)
	return reconcile.Result{}, nil
}

// Helper to reconcile a Secret.
func (r *DataSyncerReconciler) reconcileSecret(ctx context.Context, secret *corev1.Secret) (reconcile.Result, error) {
	// Parse annotation and sync to target namespaces
	challengeNames, err := getChallengeNamesFromAnnotation(secret)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to parse challenges annotation: %w", err)
	}
	for _, challengeName := range challengeNames {
		if err := r.syncSecretToNamespace(ctx, secret, challengeName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to sync secret to namespace %s: %w", challengeName, err)
		}
	}
	return reconcile.Result{}, nil
}

// Helper to reconcile a ConfigMap.
func (r *DataSyncerReconciler) reconcileConfigMap(ctx context.Context, configMap *corev1.ConfigMap) (reconcile.Result, error) {
	// Parse annotation and sync to target namespaces
	challengeNames, err := getChallengeNamesFromAnnotation(configMap)
	if err != nil {
		return reconcile.Result{}, fmt.Errorf("failed to parse challenges annotation: %w", err)
	}
	for _, challengeName := range challengeNames {
		if err := r.syncConfigMapToNamespace(ctx, configMap, challengeName); err != nil {
			return reconcile.Result{}, fmt.Errorf("failed to sync configmap to namespace %s: %w", challengeName, err)
		}
	}
	return reconcile.Result{}, nil
}

func (r *DataSyncerReconciler) syncSecretToNamespace(ctx context.Context, secret *corev1.Secret, targetNamespace string) error {
	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secret.Name,
			Namespace: targetNamespace,
		},
		Data:       secret.Data,
		StringData: secret.StringData,
		Type:       secret.Type,
	}
	return r.createOrUpdate(ctx, newSecret)
}

func (r *DataSyncerReconciler) syncConfigMapToNamespace(ctx context.Context, configMap *corev1.ConfigMap, targetNamespace string) error {
	newConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMap.Name,
			Namespace: targetNamespace,
		},
		Data:       configMap.Data,
		BinaryData: configMap.BinaryData,
	}
	return r.createOrUpdate(ctx, newConfigMap)
}

func (r *DataSyncerReconciler) createOrUpdate(ctx context.Context, obj ctrlruntimeclient.Object) error {
	r.log.Info("Syncing the data object", "object", obj.GetObjectKind())
	err := r.Create(ctx, obj)
	if err != nil && apierrors.IsAlreadyExists(err) {
		return r.Update(ctx, obj)
	}
	return err
}

// Helper to parse challenge names from annotation.
func getChallengeNamesFromAnnotation(obj ctrlruntimeclient.Object) ([]string, error) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return nil, fmt.Errorf("no annotations found")
	}
	annotationValue, exists := annotations[challenge.DataObjectAnnotationKey]
	if !exists {
		return nil, fmt.Errorf("no challenges annotation found")
	}
	var challengeNames []string
	if err := json.Unmarshal([]byte(annotationValue), &challengeNames); err != nil {
		return nil, fmt.Errorf("failed to parse challenges annotation: %w", err)
	}
	return challengeNames, nil
}
