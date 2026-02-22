//go:build e2e
// +build e2e

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
	"fmt"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"
	consumercontroller "github.com/kubeflag/kubeflag/pkg/controllers/consumer"
	kubeflaglog "github.com/kubeflag/kubeflag/pkg/log"
	"github.com/kubeflag/kubeflag/pkg/test/e2e/suite"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

const (
	tokenNamespace = "kubeflag-system"
	timeout        = 30 * time.Second
	interval       = 2 * time.Second
)

func TestConsumerE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Consumer Controller E2E Suite")
}

var _ = BeforeSuite(func() {
	mgr, ctx, cancel = suite.Bootstrap()

	go func() {
		defer GinkgoRecover()
		Expect(consumercontroller.Add(ctx, mgr, 1, &log)).To(Succeed())
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	suite.Teardown(cancel)
})

// newTenant builds a minimal Tenant for test use.
func newTenant(name string) *kubeflagv1.Tenant {
	return &kubeflagv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kubeflagv1.TenantSpec{
			Policies: kubeflagv1.TenantPolicies{
				MaxInstancesPerUser: 3,
				MaxInstancesPerTeam: 10,
			},
		},
	}
}

// newConsumer builds an active Consumer for test use.
func newConsumer(name, tenantRef string) *kubeflagv1.Consumer {
	return &kubeflagv1.Consumer{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: kubeflagv1.ConsumerSpec{
			TenantRef: tenantRef,
		},
	}
}

// newSuspendedConsumer builds a Consumer that starts in Suspended phase.
func newSuspendedConsumer(name, tenantRef string) *kubeflagv1.Consumer {
	c := newConsumer(name, tenantRef)
	c.Spec.Suspended = true
	return c
}

// tokenSecretName mirrors the naming convention used by the controller.
func tokenSecretName(consumerName string) string {
	return consumerName + "-token"
}

// decodeJWTClaims parses the JWT payload without verifying the signature.
// Sufficient for E2E claim assertions; cryptographic correctness is a unit test concern.
func decodeJWTClaims(tokenStr string) (consumercontroller.KubeflagClaims, error) {
	var claims consumercontroller.KubeflagClaims
	_, _, err := jwt.NewParser().ParseUnverified(tokenStr, &claims)
	return claims, err
}

// waitForPhase blocks until the Consumer reaches the expected phase.
func waitForPhase(consumerName string, phase kubeflagv1.ConsumerPhase) {
	GinkgoHelper()
	Eventually(func() (kubeflagv1.ConsumerPhase, error) {
		var c kubeflagv1.Consumer
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: consumerName}, &c); err != nil {
			return "", err
		}
		return c.Status.Phase, nil
	}, timeout, interval).Should(Equal(phase))
}

// waitForTokenSecret blocks until the token Secret exists in kubeflag-system.
func waitForTokenSecret(consumerName string) {
	GinkgoHelper()
	Eventually(func() error {
		var s corev1.Secret
		return k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(consumerName), Namespace: tokenNamespace}, &s)
	}, timeout, interval).Should(Succeed())
}

// ────────────────────────────────────────────────────────────────────────────────
// Token Issuance
// ────────────────────────────────────────────────────────────────────────────────

var _ = Describe("Token Issuance", func() {

	It("issues a JWT with correct claims for an Active Consumer", func() {
		name := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
		tenantName := fmt.Sprintf("tenant-%d", time.Now().UnixNano())

		tenant := newTenant(tenantName)
		consumer := newConsumer(name, tenantName)

		By("creating the Tenant")
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, tenant) })

		By("creating the Consumer")
		Expect(k8sClient.Create(ctx, consumer)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, consumer) })

		By("waiting for phase Active and token Secret")
		waitForPhase(name, kubeflagv1.ConsumerPhaseActive)
		waitForTokenSecret(name)

		By("verifying status fields are populated")
		var got kubeflagv1.Consumer
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, &got)).To(Succeed())
		Expect(got.Status.TokenSecretRef).NotTo(BeNil())
		Expect(got.Status.TokenSecretRef.Name).To(Equal(tokenSecretName(name)))
		Expect(got.Status.TokenSecretRef.Namespace).To(Equal(tokenNamespace))
		Expect(got.Status.IssuedAt).NotTo(BeNil())

		By("decoding the JWT and asserting claims")
		var secret corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &secret)).To(Succeed())
		tokenStr := string(secret.Data["token"])
		Expect(tokenStr).NotTo(BeEmpty())

		claims, err := decodeJWTClaims(tokenStr)
		Expect(err).NotTo(HaveOccurred())
		Expect(claims.Issuer).To(Equal("kubeflag.io"))
		Expect(claims.Subject).To(Equal("consumer:" + name))
		Expect(claims.ID).NotTo(BeEmpty())
		Expect(claims.Tenant).To(Equal(tenantName))
		Expect(claims.CID).To(Equal(name))
		Expect(claims.ExpiresAt).NotTo(BeNil())
		Expect(claims.ExpiresAt.Time).To(BeTemporally(">", time.Now()))
	})

	It("does not issue a token for a Suspended Consumer", func() {
		name := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
		tenantName := fmt.Sprintf("tenant-%d", time.Now().UnixNano())

		tenant := newTenant(tenantName)
		consumer := newSuspendedConsumer(name, tenantName)

		By("creating the Tenant")
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, tenant) })

		By("creating the Suspended Consumer")
		Expect(k8sClient.Create(ctx, consumer)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, consumer) })

		By("waiting for phase Suspended")
		waitForPhase(name, kubeflagv1.ConsumerPhaseSuspended)

		By("verifying no token Secret is created")
		Consistently(func() bool {
			var s corev1.Secret
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &s))
		}, 8*time.Second, interval).Should(BeTrue())
	})
})

// ────────────────────────────────────────────────────────────────────────────────
// Self-Healing
// ────────────────────────────────────────────────────────────────────────────────

var _ = Describe("Self-Healing", func() {

	It("reissues the token Secret when it is manually deleted", func() {
		name := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
		tenantName := fmt.Sprintf("tenant-%d", time.Now().UnixNano())

		tenant := newTenant(tenantName)
		consumer := newConsumer(name, tenantName)

		By("creating the Tenant and Consumer")
		Expect(k8sClient.Create(ctx, tenant)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, tenant) })
		Expect(k8sClient.Create(ctx, consumer)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, consumer) })

		By("waiting for the initial token Secret")
		waitForPhase(name, kubeflagv1.ConsumerPhaseActive)
		waitForTokenSecret(name)

		By("reading the original JWT ID")
		var original corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &original)).To(Succeed())
		originalClaims, err := decodeJWTClaims(string(original.Data["token"]))
		Expect(err).NotTo(HaveOccurred())

		By("deleting the token Secret")
		Expect(k8sClient.Delete(ctx, &original)).To(Succeed())

		By("waiting for the token Secret to be fully removed")
		Eventually(func() bool {
			var s corev1.Secret
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &s))
		}, timeout, interval).Should(BeTrue())

		By("waiting for the controller to reissue a new token Secret")
		waitForTokenSecret(name)

		By("verifying the new JWT has a different ID (genuinely re-issued)")
		var reissued corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &reissued)).To(Succeed())
		reissuedClaims, err := decodeJWTClaims(string(reissued.Data["token"]))
		Expect(err).NotTo(HaveOccurred())
		Expect(reissuedClaims.ID).NotTo(Equal(originalClaims.ID))
	})
})

// ────────────────────────────────────────────────────────────────────────────────
// Phase Transitions
// ────────────────────────────────────────────────────────────────────────────────

var _ = Describe("Phase Transitions", func() {

	It("keeps the token Secret when the Consumer is suspended", func() {
		name := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
		tenantName := fmt.Sprintf("tenant-%d", time.Now().UnixNano())

		By("creating the Tenant and Active Consumer")
		Expect(k8sClient.Create(ctx, newTenant(tenantName))).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &kubeflagv1.Tenant{ObjectMeta: metav1.ObjectMeta{Name: tenantName}})
		})
		Expect(k8sClient.Create(ctx, newConsumer(name, tenantName))).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &kubeflagv1.Consumer{ObjectMeta: metav1.ObjectMeta{Name: name}})
		})

		By("waiting for token to be issued")
		waitForPhase(name, kubeflagv1.ConsumerPhaseActive)
		waitForTokenSecret(name)

		By("suspending the Consumer")
		var consumer kubeflagv1.Consumer
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, &consumer)).To(Succeed())
		consumer.Spec.Suspended = true
		Expect(k8sClient.Update(ctx, &consumer)).To(Succeed())

		By("waiting for phase Suspended")
		waitForPhase(name, kubeflagv1.ConsumerPhaseSuspended)

		By("verifying the token Secret is still present")
		Consistently(func() error {
			var s corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &s)
		}, 8*time.Second, interval).Should(Succeed())
	})

	It("does not reissue the token when the Consumer is unsuspended", func() {
		name := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
		tenantName := fmt.Sprintf("tenant-%d", time.Now().UnixNano())

		By("creating the Tenant and Active Consumer")
		Expect(k8sClient.Create(ctx, newTenant(tenantName))).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &kubeflagv1.Tenant{ObjectMeta: metav1.ObjectMeta{Name: tenantName}})
		})
		Expect(k8sClient.Create(ctx, newConsumer(name, tenantName))).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &kubeflagv1.Consumer{ObjectMeta: metav1.ObjectMeta{Name: name}})
		})

		By("waiting for token to be issued")
		waitForPhase(name, kubeflagv1.ConsumerPhaseActive)
		waitForTokenSecret(name)

		By("recording the original token Secret resourceVersion")
		var original corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &original)).To(Succeed())
		originalRV := original.ResourceVersion

		By("suspending then unsuspending the Consumer")
		var consumer kubeflagv1.Consumer
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, &consumer)).To(Succeed())
		consumer.Spec.Suspended = true
		Expect(k8sClient.Update(ctx, &consumer)).To(Succeed())
		waitForPhase(name, kubeflagv1.ConsumerPhaseSuspended)

		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, &consumer)).To(Succeed())
		consumer.Spec.Suspended = false
		Expect(k8sClient.Update(ctx, &consumer)).To(Succeed())
		waitForPhase(name, kubeflagv1.ConsumerPhaseActive)

		By("verifying the token Secret was not replaced (same resourceVersion)")
		var current corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &current)).To(Succeed())
		Expect(current.ResourceVersion).To(Equal(originalRV))
	})
})

// ────────────────────────────────────────────────────────────────────────────────
// Deletion Cleanup
// ────────────────────────────────────────────────────────────────────────────────

var _ = Describe("Deletion Cleanup", func() {

	It("deletes the token Secret when the Consumer is deleted", func() {
		name := fmt.Sprintf("consumer-%d", time.Now().UnixNano())
		tenantName := fmt.Sprintf("tenant-%d", time.Now().UnixNano())

		By("creating the Tenant and Consumer")
		Expect(k8sClient.Create(ctx, newTenant(tenantName))).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, &kubeflagv1.Tenant{ObjectMeta: metav1.ObjectMeta{Name: tenantName}})
		})
		consumer := newConsumer(name, tenantName)
		Expect(k8sClient.Create(ctx, consumer)).To(Succeed())

		By("waiting for the token Secret to be issued")
		waitForPhase(name, kubeflagv1.ConsumerPhaseActive)
		waitForTokenSecret(name)

		By("deleting the Consumer")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name}, consumer)).To(Succeed())
		Expect(k8sClient.Delete(ctx, consumer)).To(Succeed())

		By("verifying the Consumer is fully removed")
		Eventually(func() bool {
			var c kubeflagv1.Consumer
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: name}, &c))
		}, timeout, interval).Should(BeTrue())

		By("verifying the token Secret is also deleted")
		Eventually(func() bool {
			var s corev1.Secret
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: tokenSecretName(name), Namespace: tokenNamespace}, &s))
		}, timeout, interval).Should(BeTrue())
	})
})
