/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGitHubCredentialResolverResolvePATSecret(t *testing.T) {
	resolver := newTestCredentialResolver(t,
		testWorkload("runner-workload", "github-secret"),
		testSecret("github-secret", map[string][]byte{
			gitHubTokenKey: []byte("pat-token"),
		}),
	)

	credential, err := resolver.Resolve(context.Background(), &WorkflowRunRecord{WorkloadID: "runner-workload"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if credential.Type != gitHubAuthTypePAT || credential.Token != "pat-token" {
		t.Fatalf("Resolve() credential = %#v, want PAT token", credential)
	}
}

func TestGitHubCredentialResolverResolveAppSecret(t *testing.T) {
	resolver := newTestCredentialResolver(t,
		testWorkload("runner-workload", "github-app-secret"),
		testSecret("github-app-secret", map[string][]byte{
			gitHubAppIDKey:             []byte("123"),
			gitHubAppInstallationIDKey: []byte("456"),
			gitHubAppPrivateKeyKey:     []byte("private-key"),
		}),
	)

	credential, err := resolver.Resolve(context.Background(), &WorkflowRunRecord{WorkloadID: "runner-workload"})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if credential.Type != gitHubAuthTypeApp ||
		credential.AppID != "123" ||
		credential.InstallationID != "456" ||
		credential.PrivateKey != "private-key" {
		t.Fatalf("Resolve() credential = %#v, want GitHub App credential", credential)
	}
}

func TestGitHubCredentialResolverMissingAnnotationDoesNotUseArbitrarySecret(t *testing.T) {
	resolver := newTestCredentialResolver(t,
		&v1.Workload{
			ObjectMeta: metav1.ObjectMeta{Name: "runner-workload"},
		},
		testSecret("unrelated-secret", map[string][]byte{
			gitHubTokenKey: []byte("wrong-token"),
		}),
	)

	_, err := resolver.Resolve(context.Background(), &WorkflowRunRecord{WorkloadID: "runner-workload"})
	if err == nil {
		t.Fatal("Resolve() error = nil, want missing annotation error")
	}
	if !strings.Contains(err.Error(), "no github secret annotation") {
		t.Fatalf("Resolve() error = %v, want missing annotation error", err)
	}
}

func TestGitHubTokenSourcePAT(t *testing.T) {
	token, err := NewGitHubTokenSource().Token(context.Background(), &GitHubCredential{
		Type:  gitHubAuthTypePAT,
		Token: "pat-token",
	})
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token != "pat-token" {
		t.Fatalf("Token() = %q, want pat-token", token)
	}
}

func TestGitHubTokenSourceGitHubApp(t *testing.T) {
	privateKey := testRSAPrivateKeyPEM(t)
	requestSeen := make(chan struct{}, 1)
	handlerErr := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case requestSeen <- struct{}{}:
		default:
		}
		recordHandlerError := func(message string) {
			select {
			case handlerErr <- message:
			default:
			}
			http.Error(w, message, http.StatusBadRequest)
		}

		if r.Method != http.MethodPost {
			recordHandlerError("method = " + r.Method + ", want POST")
			return
		}
		if r.URL.Path != "/app/installations/456/access_tokens" {
			recordHandlerError("path = " + r.URL.Path + ", want installation token path")
			return
		}
		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "Bearer ") {
			recordHandlerError("Authorization header does not contain bearer JWT")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token":"installation-token"}`))
	}))
	defer server.Close()

	source := NewGitHubTokenSource()
	source.baseURL = server.URL
	source.httpClient = server.Client()
	source.now = func() time.Time {
		return time.Unix(1700000000, 0)
	}

	token, err := source.Token(context.Background(), &GitHubCredential{
		Type:           gitHubAuthTypeApp,
		AppID:          "123",
		InstallationID: "456",
		PrivateKey:     privateKey,
	})
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	select {
	case message := <-handlerErr:
		t.Fatal(message)
	default:
	}
	select {
	case <-requestSeen:
	default:
		t.Fatal("Token() did not call installation token endpoint")
	}
	if token != "installation-token" {
		t.Fatalf("Token() = %q, want installation-token", token)
	}
}

func newTestCredentialResolver(t *testing.T, objects ...client.Object) *GitHubCredentialResolver {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1.AddToScheme() error = %v", err)
	}
	if err := v1.AddToScheme(scheme); err != nil {
		t.Fatalf("v1.AddToScheme() error = %v", err)
	}

	return NewGitHubCredentialResolver(fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build())
}

func testWorkload(name, secretName string) *v1.Workload {
	return &v1.Workload{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				v1.GithubSecretIdAnnotation: secretName,
			},
		},
	}
}

func testSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: common.PrimusSafeNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
}

func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}
	return string(pem.EncodeToMemory(block))
}
