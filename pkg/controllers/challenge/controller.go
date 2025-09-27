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

package challenge

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	ctfv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	"github.com/kubeflag/kubeflag/pkg/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
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

const (
	ChallengeInstanceFinalizer = "kubeflag.io/cleanup-challenge-instances"
	NamespaceFinalizer         = "kubeflag.io/cleanup-challenge-namespace"
	DataObjectsAnnotation      = "sync.kubeflag.io/challenges"
	DataObjectFinalizer        = "kubeflag.io/cleanup-data-objects"
)

type ChallengeReconciler struct {
	ctrlruntimeclient.Client
	log      logr.Logger
	recorder record.EventRecorder
}

const ControllerName = "challenge-controller"

var rawLog *logr.Logger

// Add creates a new Challenge controller and adds it to the Manager.
func Add(ctx context.Context, mgr manager.Manager, numWorkers int, log *logr.Logger) error {
	reconciler := &ChallengeReconciler{
		Client:   mgr.GetClient(),
		recorder: mgr.GetEventRecorderFor(ControllerName),
	}

	// Define a custom predicate to watch ChallengeInstance resources with a matching ChallengeRef
	challengeInstancePredicate := predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return matchesChallengeRef(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return matchesChallengeRef(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return matchesChallengeRef(e.Object)
		},
	}

	// Set up the controller with the reconciler
	_, err := builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: numWorkers,
		}).
		For(&ctfv1.Challenge{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		// Watch ChallengeInstance objects and apply the custom predicate
		Watches(&ctfv1.ChallengeInstance{}, &handler.EnqueueRequestForObject{}, builder.WithPredicates(challengeInstancePredicate)).
		Owns(&corev1.Namespace{}).
		Build(reconciler)

	rawLog = log
	return err
}

func matchesChallengeRef(obj ctrlruntimeclient.Object) bool {
	instance, ok := obj.(*ctfv1.ChallengeInstance)
	if !ok {
		return false
	}
	// Check if the ChallengeRef matches the Challenge Name
	return instance.Spec.ChallengeRef == obj.GetName()
}

func (r *ChallengeReconciler) recordEvent(_ context.Context, challenge *ctfv1.Challenge, eventType, reason, message string) {
	r.log.Info(message)
	r.recorder.Event(challenge, eventType, reason, message)
}

func (r *ChallengeReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	r.log = rawLog.WithName(ControllerName).WithValues("Challenge", req.Name)
	// Fetch the Challenge instance
	challenge := &ctfv1.Challenge{}
	err := r.Get(ctx, req.NamespacedName, challenge)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Challenge resource not found. It might have been deleted.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		r.log.Error(err, "Failed to get Challenge")
		return reconcile.Result{}, err
	}

	// Add finalizer if not present
	if challenge.GetDeletionTimestamp() == nil && !kubernetes.HasFinalizer(challenge, ChallengeInstanceFinalizer) {
		kubernetes.AddFinalizer(challenge, ChallengeInstanceFinalizer)
		if err := r.Update(ctx, challenge); err != nil {
			r.log.Error(err, "Failed to add finalizer to Challenge")
			return reconcile.Result{}, err
		}
	}

	// Handle Deletion: If the Challenge is marked for deletion, delete all associated ChallengeInstances
	if challenge.GetDeletionTimestamp() != nil {
		return reconcile.Result{}, r.reconcileDelete(ctx, challenge)
	}

	// Validate the Deployment Template
	r.log.V(1).Info("Validating the Template")
	healthy := r.validateTemplate(ctx, challenge)
	challenge.Status.Healthy = healthy

	// Update Challenge status if needed
	if err := r.updateStatus(ctx, challenge); err != nil {
		r.log.Error(err, "Failed to update Challenge status")
		return reconcile.Result{}, err
	}

	// Reconcile the namespace
	if err := r.reconcileNamespace(ctx, challenge); err != nil {
		r.log.Error(err, "Failed to reconcile Namespace")
		return reconcile.Result{}, err
	}

	r.log.V(1).Info("Reconciling the References")
	if err := r.reconcileReferences(ctx, challenge); err != nil {
		return reconcile.Result{}, err
	}

	// Calculate the current hash of the PodSpec
	currentHash, err := hashTemplate(challenge.Spec.Template.Spec)
	if err != nil {
		r.log.Error(err, "Failed to calculate PodSpec hash")
		return reconcile.Result{}, err
	}

	// If TemplateHash is empty, it means this is the first reconciliation
	if challenge.Status.TemplateHash == "" {
		if err := r.updateChallengeStatus(ctx, challenge, currentHash); err != nil {
			return reconcile.Result{}, err
		}
		// Do not trigger ChallengeInstance update on initial creation
		return reconcile.Result{}, nil
	}

	// Compare with the stored hash
	if challenge.Status.TemplateHash != currentHash {
		r.log.Info("PodSpec has changed, updating ChallengeInstances")
		if err := r.updateChallengeInstances(ctx, challenge); err != nil {
			r.log.Error(err, "Failed to update ChallengeInstances")
			return reconcile.Result{}, err
		}
		// Update the status with the new hash and timestamp
		if err := r.updateChallengeStatus(ctx, challenge, currentHash); err != nil {
			return reconcile.Result{}, err
		}
	}

	return reconcile.Result{}, nil
}

func (r *ChallengeReconciler) reconcileDelete(ctx context.Context, challenge *ctfv1.Challenge) error {
	r.log.Info("Challenge is being deleted.")
	if kubernetes.HasFinalizer(challenge, ChallengeInstanceFinalizer) {
		r.log.V(1).Info("Cleaning up associated ChallengeInstances.")

		if challenge.Status.ActiveInstances > 0 {
			if err := r.cleanupChallengeInstances(ctx, challenge); err != nil {
				r.log.Error(err, "Failed to clean up ChallengeInstances")
				return err
			}
		}

		kubernetes.RemoveFinalizer(challenge, ChallengeInstanceFinalizer)
		if err := r.Update(ctx, challenge); err != nil {
			r.log.Error(err, "Failed to remove finalizer from Challenge")
			return err
		}
	}

	if kubernetes.HasFinalizer(challenge, DataObjectFinalizer) {
		if err := r.removeReferences(ctx, challenge); err != nil {
			r.log.Error(err, "Failed to clean up reference annotations")
			return err
		}
	}

	r.log.V(1).Info("Cleaning up the namespace.")
	if kubernetes.HasFinalizer(challenge, NamespaceFinalizer) {
		if err := r.cleanupChallengeNamespace(ctx, challenge); err != nil {
			r.log.Error(err, "Failed to clean up Challenge Namespace")
			return err
		}
	}
	return nil
}

func (r *ChallengeReconciler) validateTemplate(ctx context.Context, challenge *ctfv1.Challenge) bool {
	// Basic validation: Ensure the Deployment template has at least one container
	containers := challenge.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		r.recordEvent(ctx, challenge, "Warning", "ValidationFailed", "Deployment template must have at least one container.")
		return false
	}

	var containerWithPort *corev1.Container
	var containersWithPort int

	// Check each container for resource limits and container ports
	for _, container := range containers {
		if container.Name == "" {
			r.recordEvent(ctx, challenge, "Warning", "ValidationFailed", "Container name cannot be empty.")
			return false
		}
		if challenge.Spec.ExposedContainerName != "" && container.Name == challenge.Spec.ExposedContainerName && len(container.Ports) == 0 {
			r.recordEvent(ctx, challenge, "Warning", "ValidationFailed", "Exposed container must have a container port!")
			return false
		}
		if container.Image == "" {
			r.recordEvent(ctx, challenge, "Warning", "ValidationFailed", "Container image cannot be empty.")
			return false
		}
		if container.Resources.Limits == nil || container.Resources.Requests == nil {
			r.recordEvent(ctx, challenge, "Warning", "ValidationFailed", fmt.Sprintf("Container '%s' must have resource limits and requests defined.", container.Name))
			return false
		}

		if len(container.Ports) > 0 {
			containerWithPort = &container
			containersWithPort++
		}
	}

	// Validate If we have a container to expose
	if containersWithPort == 0 {
		r.recordEvent(ctx, challenge, "Warning", "ValidationFailed", "No container with a defined port found. The challenge cannot be exposed.")
		return false
	}
	// Validate ExposedContainerName if provided
	if challenge.Spec.ExposedContainerName == "" {
		if containersWithPort > 1 {
			r.recordEvent(ctx, challenge, "Warning", "ValidationFailed", "Multiple containers with ports are defined. Please specify the 'ExposedContainerName'.")
			return false
		}

		// Automatically set the exposed container name
		challenge.Spec.ExposedContainerName = containerWithPort.Name
		if err := r.Update(ctx, challenge); err != nil {
			r.log.Error(err, "Failed to set the default exposed container.")
			r.recordEvent(ctx, challenge, "Warning", "ValidationError", fmt.Sprintf("Failed to set the default exposed container: %v", err))
			return false
		}
	}

	// If all validations pass
	r.recordEvent(ctx, challenge, "Normal", "ValidationSucceeded", "Challenge template validation succeeded.")
	return true
}

func (r *ChallengeReconciler) updateStatus(ctx context.Context, challenge *ctfv1.Challenge) error {
	// List all ChallengeInstances associated with this Challenge
	var instances ctfv1.ChallengeInstanceList
	labelSelector := labels.SelectorFromSet(map[string]string{"challengeRef": challenge.Name})
	if err := r.List(ctx, &instances, &ctrlruntimeclient.ListOptions{
		Namespace:     challenge.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return err
	}

	// Update the status
	challenge.Status.ActiveInstances = len(instances.Items)
	challenge.Status.LastUpdated = metav1.Now()

	return r.Status().Update(ctx, challenge)
}

func (r *ChallengeReconciler) cleanupChallengeInstances(ctx context.Context, challenge *ctfv1.Challenge) error {
	var instances ctfv1.ChallengeInstanceList
	labelSelector := labels.SelectorFromSet(map[string]string{"challengeRef": challenge.Name})
	if err := r.List(ctx, &instances, &ctrlruntimeclient.ListOptions{
		Namespace:     challenge.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return err
	}

	for _, instance := range instances.Items {
		err := r.Delete(ctx, &instance)
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func (r *ChallengeReconciler) cleanupChallengeNamespace(ctx context.Context, challenge *ctfv1.Challenge) error {
	// Fetch the namespace associated with the Challenge
	namespace := &corev1.Namespace{}
	err := r.Get(ctx, ctrlruntimeclient.ObjectKey{Name: challenge.Name}, namespace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace already deleted or doesn't exist, nothing to do
			r.log.Info("Namespace not found, might already be deleted", "namespace", challenge.Name)
			return nil
		}
		// If another error occurred, return it
		return err
	}

	// Optional: Check if the Namespace has a specific label created by the ChallengeReconciler
	if _, ok := namespace.Labels["challengeRef"]; !ok {
		r.log.Info("Namespace does not have challengeRef label, skipping deletion", "namespace", challenge.Name)
		return nil
	}

	// Namespace exists, delete it
	if err := r.Delete(ctx, namespace); err != nil {
		r.log.Error(err, "Failed to delete Namespace", "namespace", challenge.Name)
		return err
	}

	kubernetes.RemoveFinalizer(challenge, NamespaceFinalizer)
	if err := r.Update(ctx, challenge); err != nil {
		r.log.Error(err, "Failed to remove the namespace finalizer from Challenge")
		return err
	}

	r.log.Info("Deleted Namespace", "namespace", challenge.Name)
	return nil
}

func (r *ChallengeReconciler) updateChallengeInstances(ctx context.Context, challenge *ctfv1.Challenge) error {
	var instances ctfv1.ChallengeInstanceList
	labelSelector := labels.SelectorFromSet(map[string]string{"challengeRef": challenge.Name})
	if err := r.List(ctx, &instances, &ctrlruntimeclient.ListOptions{
		Namespace:     challenge.Namespace,
		LabelSelector: labelSelector,
	}); err != nil {
		return err
	}

	for _, instance := range instances.Items {
		resourceName := fmt.Sprintf("%s-%s", challenge.Name, instance.Spec.User)
		// Fetch and delete the associated Deployment
		deployment := &appsv1.Deployment{}
		err := r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: challenge.Name}, deployment)
		if err == nil {
			if err := r.Delete(ctx, deployment); err != nil {
				return err
			}
			r.log.Info("Deleted Deployment to allow ChallengeInstance controller to recreate it", "deployment", deployment.Name)
		} else if !apierrors.IsNotFound(err) {
			// If there's an error other than NotFound, return it
			return err
		}

		// Fetch and delete the associated Service
		service := &corev1.Service{}
		err = r.Get(ctx, types.NamespacedName{Name: resourceName, Namespace: challenge.Name}, service)
		if err == nil {
			if err := r.Delete(ctx, service); err != nil {
				return err
			}
			r.log.Info("Deleted Service to allow ChallengeInstance controller to recreate it", "service", service.Name)
		} else if !apierrors.IsNotFound(err) {
			// If there's an error other than NotFound, return it
			return err
		}
	}

	return nil
}

func (r *ChallengeReconciler) updateChallengeStatus(ctx context.Context, challenge *ctfv1.Challenge, hash string) error {
	challenge.Status.TemplateHash = hash
	challenge.Status.LastUpdated = metav1.Now()
	if err := r.Status().Update(ctx, challenge); err != nil {
		r.log.Error(err, "Failed to update Challenge status")
		return err
	}
	return nil
}

func (r *ChallengeReconciler) reconcileNamespace(ctx context.Context, challenge *ctfv1.Challenge) error {
	// Check if the namespace with the same name as the Challenge already exists
	namespace := &corev1.Namespace{}
	err := r.Get(ctx, ctrlruntimeclient.ObjectKey{Name: challenge.Name}, namespace)
	if err != nil && apierrors.IsNotFound(err) {
		// Namespace does not exist, create it
		newNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: challenge.Name,
				Labels: map[string]string{
					"challengeRef": challenge.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(challenge, ctfv1.GroupVersion.WithKind("Challenge")),
				},
			},
		}
		if err := r.Create(ctx, newNamespace); err != nil {
			r.log.Error(err, "Failed to create Namespace", "namespace", challenge.Name)
			return err
		}
		kubernetes.AddFinalizer(challenge, NamespaceFinalizer)
		if err := r.Update(ctx, challenge); err != nil {
			r.log.Error(err, "Failed to add finalizer to Challenge")
			return err
		}
		r.log.Info("Created Namespace", "namespace", challenge.Name)
	} else if err != nil {
		// Other errors
		return err
	}
	return nil
}

func (r *ChallengeReconciler) reconcileReferences(ctx context.Context, challenge *ctfv1.Challenge) error {
	if len(challenge.Spec.SecretReferences) == 0 && len(challenge.Spec.ConfigMapReferences) == 0 {
		return nil
	}
	// Annotate Secrets
	for _, secretRef := range challenge.Spec.SecretReferences {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: secretRef.Name, Namespace: secretRef.Namespace}, secret); err == nil {
			AnnotateResourceWithChallenges(secret, challenge.Name)
			if err := r.Update(ctx, secret); err != nil {
				r.log.Error(err, "Failed to update Secret with challenges")
				return err
			}
		}
	}

	for _, configMapRef := range challenge.Spec.ConfigMapReferences {
		configMap := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: configMapRef.Name, Namespace: configMapRef.Namespace}, configMap); err == nil {
			AnnotateResourceWithChallenges(configMap, challenge.Name)
			if err := r.Update(ctx, configMap); err != nil {
				r.log.Error(err, "Failed to update ConfigMap with challenges")
				return err
			}
		}
	}

	kubernetes.AddFinalizer(challenge, DataObjectFinalizer)
	if err := r.Update(ctx, challenge); err != nil {
		r.log.Error(err, "Failed to add finalizer to Challenge")
		return err
	}
	return nil
}

func (r *ChallengeReconciler) removeReferences(ctx context.Context, challenge *ctfv1.Challenge) error {
	// Annotate Secrets
	for _, secretRef := range challenge.Spec.SecretReferences {
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: secretRef.Name, Namespace: secretRef.Namespace}, secret); err == nil {
			RemoveChallengeFromResource(secret, challenge.Name)
			if err := r.Update(ctx, secret); err != nil {
				r.log.Error(err, "Failed to update Secret with challenges")
				return err
			}
		}
	}

	for _, configMapRef := range challenge.Spec.ConfigMapReferences {
		configMap := &corev1.ConfigMap{}
		if err := r.Get(ctx, types.NamespacedName{Name: configMapRef.Name, Namespace: configMapRef.Namespace}, configMap); err == nil {
			RemoveChallengeFromResource(configMap, challenge.Name)
			if err := r.Update(ctx, configMap); err != nil {
				r.log.Error(err, "Failed to update ConfigMap with challenges")
				return err
			}
		}
	}

	kubernetes.RemoveFinalizer(challenge, DataObjectFinalizer)
	if err := r.Update(ctx, challenge); err != nil {
		r.log.Error(err, "Failed to remove finalizer from Challenge")
		return err
	}
	return nil
}
