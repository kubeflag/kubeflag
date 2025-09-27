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

package challenge

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	challengecontroller "github.com/kubeflag/kubeflag/pkg/controllers/challenge"
	kubeflaglog "github.com/kubeflag/kubeflag/pkg/log"
	"github.com/kubeflag/kubeflag/pkg/test/e2e/suite"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	mgr       manager.Manager
	cfg       *rest.Config
	log       = kubeflaglog.NewDefault()
)

// Ginkgo entry point
func TestChallengeE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Challenge Controller E2E Suite")
}

// Bootstraps a real manager and client against the Kind cluster.
var _ = BeforeSuite(func() {
	mgr, ctx, cancel = suite.Bootstrap()

	// Run controller inside a goroutine.
	go func() {
		defer GinkgoRecover()
		Expect(challengecontroller.Add(ctx, mgr, 1, &log)).To(Succeed())
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	suite.Teardown(cancel)
})

// Template Challenge used as a base for tests.
func newSampleChallenge(name string) *v1alpha1.Challenge {
	return &v1alpha1.Challenge{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.ChallengeSpec{
			DefaultTTL:           &metav1.Duration{Duration: 5 * time.Minute},
			ExposedContainerName: "challenge-container",
			Template: v1alpha1.DeploymentTemplate{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "challenge-container",
						Image: "nginx-alpine:latest",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("128Mi"),
								corev1.ResourceCPU:    resource.MustParse("100m"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("64Mi"),
								corev1.ResourceCPU:    resource.MustParse("50m"),
							},
						},
						Ports: []corev1.ContainerPort{{ContainerPort: 80, Name: "http"}},
					}},
				},
			},
		},
	}
}

var _ = Describe("Challenge Controller E2E", func() {
	const (
		timeout  = 30 * time.Second
		interval = 2 * time.Second
	)

	It("creates and deletes a Challenge with its namespace", func() {
		name := fmt.Sprintf("sample-chal-%d", time.Now().UnixNano())
		ch := newSampleChallenge(name)

		Expect(k8sClient.Create(ctx, ch)).To(Succeed())

		By("waiting for namespace creation")
		Eventually(func() error {
			var ns corev1.Namespace
			return k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name}, &ns)
		}, timeout, interval).Should(Succeed())

		By("deleting the Challenge and ensuring namespace is removed")
		Expect(k8sClient.Delete(ctx, ch)).To(Succeed())

		Eventually(func() bool {
			var ns corev1.Namespace
			err := k8sClient.Get(ctx, types.NamespacedName{Name: ch.Name}, &ns)
			return err != nil // NotFound expected
		}, timeout, interval).Should(BeTrue())
	})

	It("detects an unhealthy Challenge and marks it healthy after a fix", func() {
		name := fmt.Sprintf("unhealthy-chal-%d", time.Now().UnixNano())
		ch := newSampleChallenge(name)
		// Remove ports to trigger unhealthy state in the controller logic.
		ch.Spec.Template.Spec.Containers[0].Ports = nil

		Expect(k8sClient.Create(ctx, ch)).To(Succeed())

		By("waiting for Challenge to be marked unhealthy")
		Eventually(func() bool {
			var c v1alpha1.Challenge
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &c); err != nil {
				return false
			}
			return c.Status.Healthy == false
		}, timeout, interval).Should(BeTrue())

		By("updating the Challenge to include a port and become healthy")
		var c v1alpha1.Challenge
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: name}, &c)).To(Succeed())
		c.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{ContainerPort: 80, Name: "http"}}
		time.Sleep(2 * time.Second)
		Expect(k8sClient.Update(ctx, &c)).To(Succeed())

		Eventually(func() bool {
			var updated v1alpha1.Challenge
			if err := k8sClient.Get(ctx, types.NamespacedName{Name: name}, &updated); err != nil {
				return false
			}
			return updated.Status.Healthy
		}, timeout, interval).Should(BeTrue())

		By("cleaning up the Challenge")
		Expect(k8sClient.Delete(ctx, ch)).To(Succeed())

		// Wait for the Challenge object itself to be fully removed
		Eventually(func() bool {
			tmp := &v1alpha1.Challenge{}
			err := k8sClient.Get(ctx, client.ObjectKey{
				Name:      ch.Name,
				Namespace: ch.Namespace,
			}, tmp)
			return apierrors.IsNotFound(err)
		}, 2*time.Minute, 2*time.Second).Should(BeTrue(), "Challenge should be deleted")
	})
})
