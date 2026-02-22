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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	kubeflagv1 "github.com/kubeflag/kubeflag/pkg/api/v1alpha1"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	signingKeySecretName      = "kubeflag-signing-key"
	signingKeySecretNamespace = "kubeflag-system"

	// defaultTokenTTL is used when spec.expiresAt is not set.
	defaultTokenTTL = 365 * 24 * time.Hour
)

// KubeflagClaims is the JWT payload issued for a Consumer.
type KubeflagClaims struct {
	jwt.RegisteredClaims

	// Tenant is the tenantRef from the Consumer spec.
	Tenant string `json:"tenant"`

	// CID is the Consumer's metadata.name — used by the API to look up the Consumer.
	CID string `json:"cid"`
}

// EnsureSigningKey returns the RSA private key and key-ID stored in the
// kubeflag-signing-key Secret, creating a fresh RSA-2048 key pair if the
// Secret does not exist yet.
func EnsureSigningKey(ctx context.Context, c ctrlruntimeclient.Client) (*rsa.PrivateKey, string, error) {
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      signingKeySecretName,
		Namespace: signingKeySecretNamespace,
	}, secret)

	if err == nil {
		return parseSigningKeySecret(secret)
	}

	if !apierrors.IsNotFound(err) {
		return nil, "", fmt.Errorf("get signing-key secret: %w", err)
	}

	// Secret does not exist — generate a new RSA-2048 key pair.
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, "", fmt.Errorf("generate RSA key: %w", err)
	}

	privDER, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, "", fmt.Errorf("marshal private key: %w", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})

	pubDER, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	if err != nil {
		return nil, "", fmt.Errorf("marshal public key: %w", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	kid := uuid.New().String()

	newSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      signingKeySecretName,
			Namespace: signingKeySecretNamespace,
		},
		Data: map[string][]byte{
			"private.pem": privPEM,
			"public.pem":  pubPEM,
			"kid":         []byte(kid),
		},
	}

	if err := c.Create(ctx, newSecret); err != nil {
		// Handle a race where another replica created it first.
		if apierrors.IsAlreadyExists(err) {
			if err := c.Get(ctx, types.NamespacedName{
				Name:      signingKeySecretName,
				Namespace: signingKeySecretNamespace,
			}, secret); err != nil {
				return nil, "", fmt.Errorf("get signing-key secret after conflict: %w", err)
			}
			return parseSigningKeySecret(secret)
		}
		return nil, "", fmt.Errorf("create signing-key secret: %w", err)
	}

	return privKey, kid, nil
}

// parseSigningKeySecret reads private.pem and kid from an existing Secret.
func parseSigningKeySecret(secret *corev1.Secret) (*rsa.PrivateKey, string, error) {
	privPEM, ok := secret.Data["private.pem"]
	if !ok {
		return nil, "", fmt.Errorf("signing-key secret missing private.pem")
	}
	block, _ := pem.Decode(privPEM)
	if block == nil {
		return nil, "", fmt.Errorf("signing-key secret: invalid PEM in private.pem")
	}
	raw, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse private key: %w", err)
	}
	rsaKey, ok := raw.(*rsa.PrivateKey)
	if !ok {
		return nil, "", fmt.Errorf("signing key is not an RSA key")
	}
	kid := string(secret.Data["kid"])
	return rsaKey, kid, nil
}

// IssueToken mints a signed RS256 JWT for the given Consumer.
// The exp claim is set to spec.expiresAt when provided, otherwise now+365d.
func IssueToken(key *rsa.PrivateKey, kid string, consumer *kubeflagv1.Consumer) (string, error) {
	now := time.Now()
	exp := now.Add(defaultTokenTTL)
	if consumer.Spec.ExpiresAt != nil {
		exp = consumer.Spec.ExpiresAt.Time
	}

	claims := KubeflagClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "kubeflag.io",
			Subject:   "consumer:" + consumer.Name,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
			ID:        uuid.New().String(),
		},
		Tenant: consumer.Spec.TenantRef,
		CID:    consumer.Name,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	return token.SignedString(key)
}
