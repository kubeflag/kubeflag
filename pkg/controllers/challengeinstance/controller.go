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

package challengeinstance

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"go.uber.org/zap"

	v1alpha1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	"github.com/kubeflag/kubeflag/pkg/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// DeploymentFinalizer will instruct the deletion of the deployment.
	DeployementFinalizer = "kubeflag.io/cleanup-deployment"

	InstanceFinalizer = "kubeflag.io/cleanup-instance"

	ServiceFinalizer = "kubeflag.io/cleanup-service"

	ControllerName = "challenge-instance-controller"
)

type ChallengeInstanceReconciler struct {
	ctrlruntimeclient.Client
	log      *zap.SugaredLogger
	recorder record.EventRecorder
	events   chan event.GenericEvent
	timers   map[string]*time.Timer // Map t
}

// Add creates a new ChallengeInstance controller and adds it to the Manager.
func Add(ctx context.Context, mgr manager.Manager, numWorkers int, log *zap.SugaredLogger) error {
	reconciler := &ChallengeInstanceReconciler{
		Client:   mgr.GetClient(),
		log:      log,
		recorder: mgr.GetEventRecorderFor(ControllerName),
		timers:   make(map[string]*time.Timer),
		events:   make(chan event.GenericEvent),
	}
	// Set up the controller with the reconciler
	_, err := builder.ControllerManagedBy(mgr).
		Named(ControllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: numWorkers,
		}).
		For(&v1alpha1.ChallengeInstance{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&appsv1.Deployment{}). // Watch Deployment resources owned by ChallengeInstance
		Owns(&corev1.Service{}).    // Watch Service resources owned by ChallengeInstance
		WatchesRawSource(source.Channel(reconciler.events, &handler.EnqueueRequestForObject{})).
		Build(reconciler)

	return err
}

func (r *ChallengeInstanceReconciler) recordEvent(_ context.Context, challengeInstance *v1alpha1.ChallengeInstance, eventType, reason, message string) {
	r.log.Info(message, "Challenge Instance", challengeInstance.Name)
	r.recorder.Event(challengeInstance, eventType, reason, message)
}

func (r *ChallengeInstanceReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	log := r.log.With("challengeinstance", req.NamespacedName)
	log.Debug("Reconciling ChallengeInstance")

	// Fetch the ChallengeInstance
	instance := &v1alpha1.ChallengeInstance{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Instance not found, maybe deleted
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	// Fetch the associated Challenge
	challenge := &v1alpha1.Challenge{}
	if err := r.Get(ctx, ctrlruntimeclient.ObjectKey{Name: instance.Spec.ChallengeRef}, challenge); err != nil {
		log.Error(err, "Failed to get associated Challenge")
		return reconcile.Result{}, err
	}

	// Ensure the Challenge is healthy before proceeding
	// TODO: webhook check if challenge is healthy.
	if !challenge.Status.Healthy {
		log.Warn("Associated Challenge is not healthy, skipping instance creation")
		return reconcile.Result{}, nil
	}

	r.recordEvent(ctx, instance, "Normal", "InstanceValidated", "Challenge is healthy.")

	// Handle Deletion: If the Challenge is marked for deletion, delete all associated ChallengeInstances
	if instance.GetDeletionTimestamp() != nil {
		if kubernetes.HasFinalizer(instance, InstanceFinalizer) {
			log.Info("Instance is being deleted. Cleaning up associated Resources.")
			kubernetes.RemoveFinalizer(instance, InstanceFinalizer)
			if err := r.Update(ctx, instance); err != nil {
				r.log.Error(err, "Failed to add finalizer to Challenge Instance")
				return reconcile.Result{}, err
			}
		}
		return reconcile.Result{}, nil
	}

	// Add finalizer if not present
	if instance.GetDeletionTimestamp() == nil && !kubernetes.HasFinalizer(instance, InstanceFinalizer) {
		kubernetes.AddFinalizer(instance, InstanceFinalizer)
		if err := r.Update(ctx, instance); err != nil {
			r.log.Error(err, "Failed to add finalizer to Challenge Instance")
			return reconcile.Result{}, err
		}
	}

	// Set the challengeRef label for Challenge Instance
	labels := map[string]string{
		"challengeRef": challenge.Name,
	}

	instance.Labels = labels
	if err := r.Update(ctx, instance); err != nil {
		r.log.Error(err, "Failed to add labels to Challenge Instance")
		return reconcile.Result{}, err
	}

	// Calculate the expiration time based on creationTimestamp and TTL
	creationTime := instance.CreationTimestamp
	expirationTime := creationTime.Add(instance.Spec.TTL.Duration)
	instance.Status.ExpirationTime = &metav1.Time{Time: expirationTime}
	if err = r.Status().Update(ctx, instance); err != nil {
		r.log.Error(err, "Failed to update status")
		return reconcile.Result{}, err
	}

	// Check if the instance has expired
	if metav1.Now().After(expirationTime) {
		// Instance has expired, clean up the resources and delete the instance
		log.Info("ChallengeInstance has expired, cleaning up Deployment and Service")

		// Delete the ChallengeInstance
		if err := r.Delete(ctx, instance); err != nil {
			return reconcile.Result{}, err
		}

		// No need to requeue, as the instance is deleted
		return reconcile.Result{}, nil
	}

	// Manage the TTL for this instance
	r.manageInstanceTTL(instance)

	// Reconcile the Deployment
	if err := r.reconcileDeployment(ctx, instance, challenge, labels); err != nil {
		return reconcile.Result{}, err
	}

	// Reconcile the Service
	if err := r.reconcileService(ctx, instance, challenge, labels); err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func (r *ChallengeInstanceReconciler) reconcileDeployment(ctx context.Context, instance *v1alpha1.ChallengeInstance, challenge *v1alpha1.Challenge, labels map[string]string) error {
	deploymentName := fmt.Sprintf("%s-%s", challenge.Name, instance.Spec.User)
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, ctrlruntimeclient.ObjectKey{Name: deploymentName, Namespace: challenge.Name}, deployment)
	if err != nil && apierrors.IsNotFound(err) {
		// Deployment does not exist, create it
		deployment = r.newDeployment(instance, challenge)
		if err = r.Create(ctx, deployment); err != nil {
			r.log.Error(err, "Failed to create Deployment")
			return err
		}

		r.log.Info("Created Deployment", "deployment", deploymentName)
		return nil
	} else if err != nil {
		return err
	}

	// Compare and update the Deployment if it doesn't match the desired state
	desiredDeployment := r.newDeployment(instance, challenge)
	if !r.deploymentEqual(deployment, desiredDeployment) {
		r.log.Info("Deployment does not match desired state, updating", "deployment", deploymentName)
		deployment.Spec = desiredDeployment.Spec
		if err := r.Update(ctx, deployment); err != nil {
			r.log.Error(err, "Failed to update Deployment")
			return err
		}
	}

	return nil
}

func (r *ChallengeInstanceReconciler) newDeployment(instance *v1alpha1.ChallengeInstance, challenge *v1alpha1.Challenge) *appsv1.Deployment {
	// Set the challengeRef label for Deployment and Service
	labels := map[string]string{
		"instance": fmt.Sprintf("%s-%s", challenge.Name, instance.Spec.User),
	}
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", challenge.Name, instance.Spec.User),
			Namespace: challenge.Name,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, v1alpha1.GroupVersion.WithKind("ChallengeInstance")),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: challenge.Spec.Template.Spec, // Use the PodSpec from the Challenge
			},
		},
	}
}

func (r *ChallengeInstanceReconciler) reconcileService(ctx context.Context, instance *v1alpha1.ChallengeInstance, challenge *v1alpha1.Challenge, labels map[string]string) error {
	serviceName := fmt.Sprintf("%s-%s", challenge.Name, instance.Spec.User)
	service := &corev1.Service{}
	err := r.Get(ctx, ctrlruntimeclient.ObjectKey{Name: serviceName, Namespace: challenge.Name}, service)
	if err != nil && apierrors.IsNotFound(err) {
		// Service does not exist, create it
		service, err = r.newService(instance, challenge)
		if err != nil {
			r.log.Error(err, "Failed to create Service for ChallengeInstance")
			return err
		}
		if err = r.Create(ctx, service); err != nil {
			r.log.Error(err, "Failed to create Service")
			return err
		}
		r.log.Info("Created Service", "service", serviceName)
		return nil
	} else if err != nil {
		return err
	}

	// Compare and update the Service if it doesn't match the desired state
	desiredService, err := r.newService(instance, challenge)
	if err != nil {
		r.log.Error(err, "Failed to generate Service for ChallengeInstance")
		return err
	}
	if !r.serviceEqual(service, desiredService) {
		r.log.Info("Service does not match desired state, updating", "service", serviceName)
		service.Spec = desiredService.Spec
		if err := r.Update(ctx, service); err != nil {
			r.log.Error(err, "Failed to update Service")
			return err
		}
	}

	return nil
}

func (r *ChallengeInstanceReconciler) newService(instance *v1alpha1.ChallengeInstance, challenge *v1alpha1.Challenge) (*corev1.Service, error) {
	// Set the challengeRef label for Deployment and Service
	labels := map[string]string{
		"instance": fmt.Sprintf("%s-%s", challenge.Name, instance.Spec.User),
	}
	containerPort, err := getContainerPort(challenge.Spec.Template.Spec, challenge.Spec.ExposedContainerName)
	if err != nil {
		return nil, err
	}
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s", challenge.Name, instance.Spec.User),
			Namespace: challenge.Name,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(instance, v1alpha1.GroupVersion.WithKind("ChallengeInstance")),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeNodePort,
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       containerPort[0].Name,
					Port:       containerPort[0].ContainerPort,
					TargetPort: intstr.FromInt(int(containerPort[0].ContainerPort)), // Assuming the exposed container port is 8080
				},
			},
		},
	}, nil
}

// getContainerPort retrieves the containerPort for a specific container by name.
func getContainerPort(podSpec corev1.PodSpec, containerName string) ([]corev1.ContainerPort, error) {
	// Iterate over the containers in the PodSpec
	for _, container := range podSpec.Containers {
		// Check if the container name matches
		if container.Name == containerName {
			// Return the container ports
			return container.Ports, nil
		}
	}

	// If no container with the specified name was found, return an error
	return nil, fmt.Errorf("container with name %s not found", containerName)
}

// Compare the existing and desired Deployment spec.
func (r *ChallengeInstanceReconciler) deploymentEqual(existing, desired *appsv1.Deployment) bool {
	// Compare the PodTemplateSpec within the Deployment, which includes containers, volumes, etc.
	return reflect.DeepEqual(existing.Spec.Template.Spec, desired.Spec.Template.Spec) &&
		reflect.DeepEqual(existing.Spec.Replicas, desired.Spec.Replicas)
}

func (r *ChallengeInstanceReconciler) serviceEqual(existing, desired *corev1.Service) bool {
	// Implement comparison logic to detect changes
	return existing.Spec.Ports[0].Port == desired.Spec.Ports[0].Port &&
		existing.Spec.Selector["instance"] == desired.Spec.Selector["instance"]
}

func (r *ChallengeInstanceReconciler) manageInstanceTTL(instance *v1alpha1.ChallengeInstance) {
	// Reconcile the challenge instance
	// Calculate the expiration time
	expirationTime := instance.Status.ExpirationTime

	// If there's an existing timer for this instance, stop and delete it
	if timer, exists := r.timers[instance.Name]; exists {
		timer.Stop()
		delete(r.timers, instance.Name)
	}

	// Calculate time until expiration
	timeUntilExpiration := time.Until(expirationTime.Time)

	// Create a new timer for the new expiration time
	timer := time.AfterFunc(timeUntilExpiration, func() {
		r.log.Infof("Expiration time reached for instance: %s, deleting resources", instance.Name)

		// Send event to trigger cleanup
		r.events <- event.GenericEvent{
			Object: instance, // Trigger a reconcile event to delete the resources
		}
	})

	// Store the timer for this instance
	r.timers[instance.Name] = timer
}
