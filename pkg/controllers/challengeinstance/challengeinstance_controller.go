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

package controller

import (
	"context"

	"github.com/go-logr/logr"

	"github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const ControllerName = "challenge-instance-controller"

// ChallengeInstanceReconciler reconciles a ChallengeInstance object.
type ChallengeInstanceReconciler struct {
	ctrlruntimeclient.Client
	log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=kubeflag.io,resources=challengeinstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=kubeflag.io,resources=challengeinstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kubeflag.io,resources=challengeinstances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the ChallengeInstance object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *ChallengeInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// Add creates a new Challenge controller and adds it to the Manager.
func Add(ctx context.Context, mgr ctrl.Manager, numWorkers int, log *logr.Logger) error {
	reconciler := &ChallengeInstanceReconciler{
		Client: mgr.GetClient(),
		log:    *log,
	}
	// Set up the controller with the reconciler
	_, err := builder.ControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: numWorkers}).
		Named(ControllerName).
		For(&v1alpha1.Challenge{}).
		Build(reconciler)

	return err
}
