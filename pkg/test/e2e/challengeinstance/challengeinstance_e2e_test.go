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

package challengeinstance

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	challengecontroller "github.com/kubeflag/kubeflag/pkg/controllers/challenge"
	instancecontroller "github.com/kubeflag/kubeflag/pkg/controllers/challengeinstance"
	kubeflaglog "github.com/kubeflag/kubeflag/pkg/log"
	"github.com/kubeflag/kubeflag/pkg/test/e2e/suite"

	"go.uber.org/zap"

	appsv1 "k8s.io/api/apps/v1"
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

	sharedChallenge *v1alpha1.Challenge
)

// Ginkgo entry point.
func TestChallengeInstanceE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ChallengeInstance Controller E2E Suite")
}

// BeforeSuite bootstraps both controllers and creates the shared Challenge.
var _ = BeforeSuite(func() {
	mgr, ctx, cancel = suite.Bootstrap()

	// The ChallengeInstance controller requires a *zap.SugaredLogger.
	zapLog, err := zap.NewDevelopment()
	Expect(err).NotTo(HaveOccurred())
	sugar := zapLog.Sugar()

	// Register Challenge and ChallengeInstance controllers.
	Expect(challengecontroller.Add(ctx, mgr, 1, &log)).To(Succeed())
	Expect(instancecontroller.Add(ctx, mgr, 1, sugar)).To(Succeed())

	// Start the manager in a goroutine.
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())

	// Create a shared Challenge that all tests reference.
	sharedChallenge = newSampleChallenge(fmt.Sprintf("e2e-chal-%d", time.Now().UnixNano()))
	Expect(k8sClient.Create(ctx, sharedChallenge)).To(Succeed())

	// Wait for the Challenge to become healthy and its namespace to exist.
	Eventually(func() bool {
		var ch v1alpha1.Challenge
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: sharedChallenge.Name}, &ch); err != nil {
			return false
		}
		return ch.Status.Healthy
	}, 60*time.Second, 2*time.Second).Should(BeTrue(), "shared Challenge should become healthy")

	Eventually(func() error {
		var ns corev1.Namespace
		return k8sClient.Get(ctx, types.NamespacedName{Name: sharedChallenge.Name}, &ns)
	}, 30*time.Second, 2*time.Second).Should(Succeed(), "challenge namespace should exist")
})

var _ = AfterSuite(func() {
	if sharedChallenge != nil {
		_ = k8sClient.Delete(context.Background(), sharedChallenge)
	}
	suite.Teardown(cancel)
})



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
						Image: "nginx:alpine",
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

func newInstance(name, user string, ttl time.Duration) *v1alpha1.ChallengeInstance {
	return &v1alpha1.ChallengeInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1alpha1.ChallengeInstanceSpec{
			ChallengeRef: sharedChallenge.Name,
			TTL:          &metav1.Duration{Duration: ttl},
			User:         user,
			Team:         "e2e-team",
		},
	}
}

// resourceName returns the Deployment/Service name the controller generates.
func resourceName(user string) string {
	return fmt.Sprintf("%s-%s", sharedChallenge.Name, user)
}

// waitForDeployment polls until the Deployment exists in the challenge namespace.
func waitForDeployment(user string, timeout time.Duration) {
	Eventually(func() error {
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      resourceName(user),
			Namespace: sharedChallenge.Name,
		}, &appsv1.Deployment{})
	}, timeout, 2*time.Second).Should(Succeed(), "Deployment should be created")
}

// waitForService polls until the Service exists in the challenge namespace.
func waitForService(user string, timeout time.Duration) {
	Eventually(func() error {
		return k8sClient.Get(ctx, types.NamespacedName{
			Name:      resourceName(user),
			Namespace: sharedChallenge.Name,
		}, &corev1.Service{})
	}, timeout, 2*time.Second).Should(Succeed(), "Service should be created")
}

// Different test cases:

var _ = Describe("ChallengeInstance Controller E2E", func() {
	const (
		timeout  = 60 * time.Second
		interval = 2 * time.Second
	)

	// ------------------------------------------------------------------
	// Test 1: TTL expiry deletes the instance and its resources
	// ------------------------------------------------------------------
	It("auto-deletes the instance and resources when TTL expires", func() {
		user := fmt.Sprintf("ttl-%d", time.Now().UnixNano())
		inst := newInstance(fmt.Sprintf("inst-ttl-%d", time.Now().UnixNano()), user, 15*time.Second)

		By("creating the ChallengeInstance with a 15s TTL")
		Expect(k8sClient.Create(ctx, inst)).To(Succeed())

		By("waiting for Deployment and Service to be created")
		waitForDeployment(user, timeout)
		waitForService(user, timeout)

		By("waiting for the TTL to expire and the instance to be deleted")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: inst.Name}, &v1alpha1.ChallengeInstance{})
			return apierrors.IsNotFound(err)
		}, 90*time.Second, interval).Should(BeTrue(), "ChallengeInstance should be deleted after TTL")

		By("verifying the Deployment is removed")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, &appsv1.Deployment{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "Deployment should be garbage-collected")

		By("verifying the Service is removed")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, &corev1.Service{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "Service should be garbage-collected")
	})

	// ------------------------------------------------------------------
	// Test 2: Deployment recovery after deletion
	// ------------------------------------------------------------------
	It("recreates the Deployment if it is deleted externally", func() {
		user := fmt.Sprintf("dep-del-%d", time.Now().UnixNano())
		inst := newInstance(fmt.Sprintf("inst-dep-del-%d", time.Now().UnixNano()), user, 5*time.Minute)

		By("creating the ChallengeInstance")
		Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		waitForDeployment(user, timeout)

		By("capturing the original Deployment UID")
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      resourceName(user),
			Namespace: sharedChallenge.Name,
		}, dep)).To(Succeed())
		originalUID := dep.UID

		By("deleting the Deployment externally")
		Expect(k8sClient.Delete(ctx, dep)).To(Succeed())

		By("waiting for the Deployment to be recreated with a new UID")
		Eventually(func() bool {
			d := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, d); err != nil {
				return false
			}
			return d.UID != originalUID
		}, timeout, interval).Should(BeTrue(), "Deployment should be recreated with a new UID")

		By("cleaning up")
		Expect(k8sClient.Delete(ctx, inst)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: inst.Name}, &v1alpha1.ChallengeInstance{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	// ------------------------------------------------------------------
	// Test 3: Deployment recovery after spec drift
	// ------------------------------------------------------------------
	It("reverts the Deployment if its spec is mutated externally", func() {
		user := fmt.Sprintf("dep-drift-%d", time.Now().UnixNano())
		inst := newInstance(fmt.Sprintf("inst-dep-drift-%d", time.Now().UnixNano()), user, 5*time.Minute)

		By("creating the ChallengeInstance")
		Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		waitForDeployment(user, timeout)

		By("mutating the Deployment image externally")
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      resourceName(user),
			Namespace: sharedChallenge.Name,
		}, dep)).To(Succeed())

		dep.Spec.Template.Spec.Containers[0].Image = "busybox:latest"
		Expect(k8sClient.Update(ctx, dep)).To(Succeed())

		By("waiting for the controller to revert the image back")
		Eventually(func() string {
			d := &appsv1.Deployment{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, d); err != nil {
				return ""
			}
			if len(d.Spec.Template.Spec.Containers) == 0 {
				return ""
			}
			return d.Spec.Template.Spec.Containers[0].Image
		}, timeout, interval).Should(Equal("nginx:alpine"), "Deployment image should be reverted")

		By("cleaning up")
		Expect(k8sClient.Delete(ctx, inst)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: inst.Name}, &v1alpha1.ChallengeInstance{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	// ------------------------------------------------------------------
	// Test 4: Service recovery after deletion
	// ------------------------------------------------------------------
	It("recreates the Service if it is deleted externally", func() {
		user := fmt.Sprintf("svc-del-%d", time.Now().UnixNano())
		inst := newInstance(fmt.Sprintf("inst-svc-del-%d", time.Now().UnixNano()), user, 5*time.Minute)

		By("creating the ChallengeInstance")
		Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		waitForService(user, timeout)

		By("capturing the original Service UID")
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      resourceName(user),
			Namespace: sharedChallenge.Name,
		}, svc)).To(Succeed())
		originalUID := svc.UID

		By("deleting the Service externally")
		Expect(k8sClient.Delete(ctx, svc)).To(Succeed())

		By("waiting for the Service to be recreated with a new UID")
		Eventually(func() bool {
			s := &corev1.Service{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, s); err != nil {
				return false
			}
			return s.UID != originalUID
		}, timeout, interval).Should(BeTrue(), "Service should be recreated with a new UID")

		By("cleaning up")
		Expect(k8sClient.Delete(ctx, inst)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: inst.Name}, &v1alpha1.ChallengeInstance{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	// ------------------------------------------------------------------
	// Test 5: Service recovery after spec drift
	// ------------------------------------------------------------------
	It("reverts the Service if its port is mutated externally", func() {
		user := fmt.Sprintf("svc-drift-%d", time.Now().UnixNano())
		inst := newInstance(fmt.Sprintf("inst-svc-drift-%d", time.Now().UnixNano()), user, 5*time.Minute)

		By("creating the ChallengeInstance")
		Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		waitForService(user, timeout)

		By("mutating the Service port externally")
		svc := &corev1.Service{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      resourceName(user),
			Namespace: sharedChallenge.Name,
		}, svc)).To(Succeed())

		svc.Spec.Ports[0].Port = 9999
		Expect(k8sClient.Update(ctx, svc)).To(Succeed())

		By("waiting for the controller to revert the port back to 80")
		Eventually(func() int32 {
			s := &corev1.Service{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, s); err != nil {
				return 0
			}
			if len(s.Spec.Ports) == 0 {
				return 0
			}
			return s.Spec.Ports[0].Port
		}, timeout, interval).Should(Equal(int32(80)), "Service port should be reverted to 80")

		By("cleaning up")
		Expect(k8sClient.Delete(ctx, inst)).To(Succeed())
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: inst.Name}, &v1alpha1.ChallengeInstance{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue())
	})

	// ------------------------------------------------------------------
	// Test 6: Instance deletion triggers full resource cleanup
	// ------------------------------------------------------------------
	It("cleans up Deployment and Service when the instance is deleted", func() {
		user := fmt.Sprintf("cleanup-%d", time.Now().UnixNano())
		inst := newInstance(fmt.Sprintf("inst-cleanup-%d", time.Now().UnixNano()), user, 5*time.Minute)

		By("creating the ChallengeInstance")
		Expect(k8sClient.Create(ctx, inst)).To(Succeed())
		waitForDeployment(user, timeout)
		waitForService(user, timeout)

		By("deleting the ChallengeInstance")
		Expect(k8sClient.Delete(ctx, inst)).To(Succeed())

		By("waiting for the ChallengeInstance to be fully removed")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: inst.Name}, &v1alpha1.ChallengeInstance{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "ChallengeInstance should be deleted")

		By("verifying the Deployment is cleaned up")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, &appsv1.Deployment{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "Deployment should be garbage-collected")

		By("verifying the Service is cleaned up")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName(user),
				Namespace: sharedChallenge.Name,
			}, &corev1.Service{})
			return apierrors.IsNotFound(err)
		}, timeout, interval).Should(BeTrue(), "Service should be garbage-collected")
	})
})
