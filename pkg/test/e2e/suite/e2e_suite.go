//go:build e2e
// +build e2e

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

package suite

import (
	"context"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// Bootstrap connects to the existing Kind cluster and returns a ready manager.
func Bootstrap() (manager.Manager, context.Context, context.CancelFunc) {
	By("Connecting to the Kind cluster")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		home, _ := os.UserHomeDir()
		kubeconfig = filepath.Join(home, ".kube", "config")
	}

	cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	Expect(err).NotTo(HaveOccurred(), "failed to load kubeconfig")

	mgr, err := manager.New(cfg, manager.Options{})
	Expect(err).NotTo(HaveOccurred(), "failed to create manager")

	// Register CRDs into scheme
	Expect(v1alpha1.AddToScheme(mgr.GetScheme())).To(Succeed())

	return mgr, ctx, cancel
}

// Teardown cancels the context, stopping the manager.
func Teardown(cancel context.CancelFunc) {
	By("Tearing down Kind suite")
	cancel()
}
