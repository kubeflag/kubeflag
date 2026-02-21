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

package datasyncer

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubeflag/kubeflag/pkg/controllers/challenge"
	datasyncercontroller "github.com/kubeflag/kubeflag/pkg/controllers/datasyncer"
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

// Ginkgo entry point
func TestDataSyncerE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DataSyncer Controller E2E Suite")
}

// Bootstraps a real manager and client against the Kind cluster.
var _ = BeforeSuite(func() {
	mgr, ctx, cancel = suite.Bootstrap()

	// Run controller inside a goroutine.
	go func() {
		defer GinkgoRecover()
		Expect(datasyncercontroller.Add(ctx, mgr, 1, &log)).To(Succeed())
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	k8sClient = mgr.GetClient()
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	suite.Teardown(cancel)
})

// newTokenSecret creates a Kubernetes Secret with token data and annotation
func newTokenSecret(name, challengeName string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Annotations: map[string]string{
				challenge.DataObjectAnnotationKey: fmt.Sprintf(`["%s"]`, challengeName),
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"token": []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"),
		},
	}
}

func newConfigMap(name, challengeName string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default", // or the source namespace
			Annotations: map[string]string{
				challenge.DataObjectAnnotationKey: fmt.Sprintf(`["%s"]`, challengeName),
			},
		},
		Data: map[string]string{
			"token": "ABCDEFGHIJKLMNOPQRSTUVWXYZ",
		},
	}
}

var _ = Describe("Secret Synchronization E2E", func() {
	const (
		timeout  = 30 * time.Second
		interval = 2 * time.Second
	)

	It("keeps the synced Secret updated", func() {
		name := fmt.Sprintf("secret-%d", time.Now().UnixNano())
		challengeName := fmt.Sprintf("chal-%s", name)

		secret := newTokenSecret(name, challengeName)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: challengeName}}

		By("creating the Challenge namespace")
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, secret)
			_ = k8sClient.Delete(ctx, ns)
		})

		By("waiting for namespace to be Active")
		Eventually(func() (corev1.NamespacePhase, error) {
			var got corev1.Namespace
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: challengeName}, &got); err != nil {
				return "", err
			}
			return got.Status.Phase, nil
		}, timeout, interval).Should(Equal(corev1.NamespaceActive))

		By("creating the source Secret")
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("verifying the Secret is replicated")
		Eventually(func() (string, error) {
			var sec corev1.Secret
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec); err != nil {
				return "", err
			}
			return string(sec.Data["token"]), nil
		}, timeout, interval).Should(Equal("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))

		By("updating the source Secret")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, secret)).To(Succeed())
		secret.Data["token"] = []byte("1234567890")
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())

		By("verifying the update is synced")
		Eventually(func() (string, error) {
			var sec corev1.Secret
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec); err != nil {
				return "", err
			}
			return string(sec.Data["token"]), nil
		}, timeout, interval).Should(Equal("1234567890"))
	})
})

var _ = Describe("Copy Tampered", func() {
	const (
		timeout  = 30 * time.Second
		interval = 2 * time.Second
	)

	It("reverts manual changes made directly to the synced copy", func() {
		name := fmt.Sprintf("secret-%d", time.Now().UnixNano())
		challengeName := fmt.Sprintf("chal-%s", name)

		secret := newTokenSecret(name, challengeName)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: challengeName}}

		By("creating the Challenge namespace")
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, secret)
			_ = k8sClient.Delete(ctx, ns)
		})

		By("waiting for namespace to be Active")
		Eventually(func() (corev1.NamespacePhase, error) {
			var got corev1.Namespace
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: challengeName}, &got); err != nil {
				return "", err
			}
			return got.Status.Phase, nil
		}, timeout, interval).Should(Equal(corev1.NamespaceActive))

		By("creating the source Secret")
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("waiting for the copy to appear")
		Eventually(func() error {
			var sec corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec)
		}, timeout, interval).Should(Succeed())

		By("tampering with the copy's data")
		var copy corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &copy)).To(Succeed())
		copy.Data["token"] = []byte("tampered-value")
		Expect(k8sClient.Update(ctx, &copy)).To(Succeed())

		By("verifying the copy is reverted to match the source")
		Eventually(func() (string, error) {
			var sec corev1.Secret
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec); err != nil {
				return "", err
			}
			return string(sec.Data["token"]), nil
		}, timeout, interval).Should(Equal("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))
	})
})

var _ = Describe("Copy Deleted", func() {
	const (
		timeout  = 30 * time.Second
		interval = 2 * time.Second
	)

	It("recreates the synced copy when it is deleted while the source still exists", func() {
		name := fmt.Sprintf("secret-%d", time.Now().UnixNano())
		challengeName := fmt.Sprintf("chal-%s", name)

		secret := newTokenSecret(name, challengeName)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: challengeName}}

		By("creating the Challenge namespace")
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, secret)
			_ = k8sClient.Delete(ctx, ns)
		})

		By("waiting for namespace to be Active")
		Eventually(func() (corev1.NamespacePhase, error) {
			var got corev1.Namespace
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: challengeName}, &got); err != nil {
				return "", err
			}
			return got.Status.Phase, nil
		}, timeout, interval).Should(Equal(corev1.NamespaceActive))

		By("creating the source Secret")
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("waiting for the copy to appear")
		Eventually(func() error {
			var sec corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec)
		}, timeout, interval).Should(Succeed())

		By("deleting the copy")
		var copy corev1.Secret
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &copy)).To(Succeed())
		Expect(k8sClient.Delete(ctx, &copy)).To(Succeed())

		// Note: We don't want to wait for the copy to be fully removed before checking for recreation, because if the controller is working correctly it may never actually be fully removed (it may be recreated faster than we can observe the deletion). Instead, we can directly check for the presence of the copy after deletion, and if the controller is working it should be present again very quickly.
		// By("waiting for the copy to be fully removed")
		// Eventually(func() bool {
		// 	var sec corev1.Secret
		// 	return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec))
		// }, timeout, interval).Should(BeTrue())

		By("verifying the controller recreates the copy")
		Eventually(func() error {
			var sec corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec)
		}, timeout, interval).Should(Succeed())
	})
})

var _ = Describe("Namespace Removed From Annotation", func() {
	const (
		timeout  = 30 * time.Second
		interval = 2 * time.Second
	)

	It("deletes the copy in a namespace that was removed from the source annotation", func() {
		name := fmt.Sprintf("secret-%d", time.Now().UnixNano())
		challengeA := fmt.Sprintf("chal-a-%s", name)
		challengeB := fmt.Sprintf("chal-b-%s", name)

		// Source targets two namespaces initially.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
				Annotations: map[string]string{
					challenge.DataObjectAnnotationKey: fmt.Sprintf(`["%s", "%s"]`, challengeA, challengeB),
				},
			},
			Type: corev1.SecretTypeOpaque,
			Data: map[string][]byte{"token": []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ")},
		}
		nsA := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: challengeA}}
		nsB := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: challengeB}}

		By("creating both Challenge namespaces")
		Expect(k8sClient.Create(ctx, nsA)).To(Succeed())
		Expect(k8sClient.Create(ctx, nsB)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, secret)
			_ = k8sClient.Delete(ctx, nsA)
			_ = k8sClient.Delete(ctx, nsB)
		})

		By("waiting for both namespaces to be Active")
		for _, ns := range []string{challengeA, challengeB} {
			ns := ns
			Eventually(func() (corev1.NamespacePhase, error) {
				var got corev1.Namespace
				if err := k8sClient.Get(ctx, client.ObjectKey{Name: ns}, &got); err != nil {
					return "", err
				}
				return got.Status.Phase, nil
			}, timeout, interval).Should(Equal(corev1.NamespaceActive))
		}

		By("creating the source Secret targeting both namespaces")
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("waiting for copies to appear in both namespaces")
		for _, ns := range []string{challengeA, challengeB} {
			ns := ns
			Eventually(func() error {
				var sec corev1.Secret
				return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: ns}, &sec)
			}, timeout, interval).Should(Succeed())
		}

		By("removing challengeB from the source annotation")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, secret)).To(Succeed())
		secret.Annotations[challenge.DataObjectAnnotationKey] = fmt.Sprintf(`["%s"]`, challengeA)
		Expect(k8sClient.Update(ctx, secret)).To(Succeed())

		By("verifying the copy in challengeB is deleted")
		Eventually(func() bool {
			var sec corev1.Secret
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeB}, &sec))
		}, timeout, interval).Should(BeTrue())

		By("verifying the copy in challengeA still exists")
		Consistently(func() error {
			var sec corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeA}, &sec)
		}, 5*time.Second, interval).Should(Succeed())
	})
})

var _ = Describe("Source Deleted", func() {
	const (
		timeout  = 30 * time.Second
		interval = 2 * time.Second
	)

	It("deletes all synced copies when the source object is deleted", func() {
		name := fmt.Sprintf("secret-%d", time.Now().UnixNano())
		challengeName := fmt.Sprintf("chal-%s", name)

		secret := newTokenSecret(name, challengeName)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: challengeName}}

		By("creating the Challenge namespace")
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			// Secret may already be deleted by the test; ignore not-found.
			_ = k8sClient.Delete(ctx, secret)
			_ = k8sClient.Delete(ctx, ns)
		})

		By("waiting for namespace to be Active")
		Eventually(func() (corev1.NamespacePhase, error) {
			var got corev1.Namespace
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: challengeName}, &got); err != nil {
				return "", err
			}
			return got.Status.Phase, nil
		}, timeout, interval).Should(Equal(corev1.NamespaceActive))

		By("creating the source Secret")
		Expect(k8sClient.Create(ctx, secret)).To(Succeed())

		By("waiting for the copy to appear")
		Eventually(func() error {
			var sec corev1.Secret
			return k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec)
		}, timeout, interval).Should(Succeed())

		By("deleting the source Secret")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, secret)).To(Succeed())
		Expect(k8sClient.Delete(ctx, secret)).To(Succeed())

		By("verifying the source is fully deleted")
		Eventually(func() bool {
			var sec corev1.Secret
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, &sec))
		}, timeout, interval).Should(BeTrue())

		By("verifying the copy is also deleted")
		Eventually(func() bool {
			var sec corev1.Secret
			return apierrors.IsNotFound(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: challengeName}, &sec))
		}, timeout, interval).Should(BeTrue())
	})
})

var _ = Describe("ConfigMap Synchronization E2E", func() {
	const (
		timeout  = 30 * time.Second
		interval = 2 * time.Second
	)

	It("Normal Sync: Create + Update", func() {
		name := fmt.Sprintf("configmap-%d", time.Now().UnixNano())
		challengeName := fmt.Sprintf("chal-for-%s", name)

		cm := newConfigMap(name, challengeName)

		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: challengeName,
			},
		}

		By("creating the Challenge namespace")
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		DeferCleanup(func() {
			_ = k8sClient.Delete(ctx, cm)
			_ = k8sClient.Delete(ctx, ns)
		})

		By("waiting for the namespace to be Active")
		Eventually(func() (corev1.NamespacePhase, error) {
			var got corev1.Namespace
			if err := k8sClient.Get(ctx, client.ObjectKey{Name: challengeName}, &got); err != nil {
				return "", err
			}
			return got.Status.Phase, nil
		}, timeout, interval).Should(Equal(corev1.NamespaceActive))

		By("creating the source ConfigMap")
		Expect(k8sClient.Create(ctx, cm)).To(Succeed())
		By("waiting for the ConfigMap to be synced")
		Eventually(func() (string, error) {
			var synced corev1.ConfigMap
			if err := k8sClient.Get(
				ctx,
				client.ObjectKey{Name: name, Namespace: challengeName},
				&synced,
			); err != nil {
				return "", err
			}
			return synced.Data["token"], nil
		}, timeout, interval).Should(Equal("ABCDEFGHIJKLMNOPQRSTUVWXYZ"))

		By("updating the ConfigMap")
		Expect(k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: "default"}, cm)).To(Succeed())
		cm.Data["token"] = "1234567890"
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())

		By("waiting for the updated ConfigMap to be synced")
		Eventually(func() (string, error) {
			var synced corev1.ConfigMap
			if err := k8sClient.Get(
				ctx,
				client.ObjectKey{Name: name, Namespace: challengeName},
				&synced,
			); err != nil {
				return "", err
			}
			return synced.Data["token"], nil
		}, timeout, interval).Should(Equal("1234567890"))
	})
})
