/*
 * Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
 * See LICENSE for license information.
 */

package workload

import (
	"slices"
	"strings"
	"testing"

	"gotest.tools/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/AMD-AIG-AIMA/SAFE/common/pkg/common"
)

func validCICDProxySecret(password []byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{v1.SecretTypeLabel: string(v1.SecretGeneral)},
			Annotations: map[string]string{v1.WorkspaceIdsAnnotation: `["workspace"]`},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{"username": []byte("proxy-user"), "password": password},
	}
}

func TestParseCICDProxy_NoProxy(t *testing.T) {
	for _, env := range []map[string]string{nil, {}, {common.ProxyUrl: ""}, {common.NoProxy: "localhost"}, {"HTTP_PROXY": "http://proxy.example.com"}} {
		config, err := ParseCICDProxy(env)
		assert.NilError(t, err)
		assert.Assert(t, config == nil)
	}
}

func TestParseCICDProxy_Endpoints(t *testing.T) {
	for _, endpoint := range []string{"http://proxy.example.com", "https://proxy.example.com/", "http://192.0.2.1:1", "http://[2001:db8::1]:65535", "https://[::1]", "http://localhost:8080"} {
		t.Run(endpoint, func(t *testing.T) {
			config, err := ParseCICDProxy(map[string]string{common.ProxyUrl: endpoint})
			assert.NilError(t, err)
			assert.Equal(t, config.URL, endpoint)
		})
	}
	for _, endpoint := range []string{"http://", "proxy.example.com", "http://%xx", " http://proxy.example.com", "http://proxy.example.com\n", "http:proxy.example.com", "ftp://proxy.example.com", "http://proxy.example.com:0", "http://proxy.example.com:65536", "http://proxy.example.com:port", "http://proxy.example.com:", "http://proxy.example.com?", "http://proxy.example.com#", "http://proxy.example.com/path", "http://2001:db8::1", "https://proxy.example.com/a%xx"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := ParseCICDProxy(map[string]string{common.ProxyUrl: endpoint})
			assert.Assert(t, err != nil)
			assert.Assert(t, strings.Contains(err.Error(), "env.PROXY_URL:"))
			assert.Assert(t, !strings.Contains(err.Error(), endpoint))
		})
	}
}

func TestParseCICDProxy_RejectsAllUserinfo(t *testing.T) {
	for _, userinfo := range []string{"sample:example", "sample", "", "%75ser:%70ass", ":"} {
		for _, secret := range []string{"", "proxy-auth"} {
			endpoint := "http://" + userinfo + "@proxy.example.com:8080"
			_, err := ParseCICDProxy(map[string]string{common.ProxyUrl: endpoint, common.ProxyCredentialSecret: secret})
			assert.Assert(t, err != nil)
			assert.Assert(t, strings.Contains(err.Error(), "userinfo is not allowed"))
			assert.Assert(t, !strings.Contains(err.Error(), endpoint))
		}
	}
}

func TestParseCICDProxy_NormalizedKeys(t *testing.T) {
	config, err := ParseCICDProxy(map[string]string{" PROXY_URL ": "http://proxy.example.com", " NO_PROXY ": " localhost "})
	assert.NilError(t, err)
	assert.Equal(t, config.URL, "http://proxy.example.com")
	assert.DeepEqual(t, config.NoProxy, []string{"localhost"})
	for _, key := range CICDProxyEnvKeys() {
		_, err = ParseCICDProxy(map[string]string{key: "", " " + key: ""})
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "env."+key+": multiple keys"))
	}
	_, err = ParseCICDProxy(map[string]string{" PROXY_URL ": "http://sample@example.com"})
	assert.Assert(t, err != nil)
}

func TestParseCICDProxy_NoProxyList(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  []string
	}{
		{"", nil}, {", ,", nil}, {"localhost, , .example.com,2001:db8::1,192.0.2.0/24,*.example.org,", []string{"localhost", ".example.com", "2001:db8::1", "192.0.2.0/24", "*.example.org"}},
	} {
		config, err := ParseCICDProxy(map[string]string{common.ProxyUrl: "http://proxy.example.com", common.NoProxy: tc.input})
		assert.NilError(t, err)
		assert.DeepEqual(t, config.NoProxy, tc.want)
	}
}

func TestParseCICDProxy_SecretName(t *testing.T) {
	boundary := strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 61)
	for _, name := range []string{"proxy-auth", boundary} {
		config, err := ParseCICDProxy(map[string]string{common.ProxyUrl: "http://proxy.example.com", common.ProxyCredentialSecret: name})
		assert.NilError(t, err)
		assert.Equal(t, config.CredentialSecret, name)
	}
	for _, name := range []string{" proxy-auth", "proxy-auth ", "workspace/proxy-auth", "https://example.com", "ProxyAuth", boundary + "x"} {
		_, err := ParseCICDProxy(map[string]string{common.ProxyUrl: "http://proxy.example.com", common.ProxyCredentialSecret: name})
		assert.Assert(t, err != nil)
		assert.Assert(t, strings.Contains(err.Error(), "env.PROXY_CREDENTIAL_SECRET:"))
	}
	_, err := ParseCICDProxy(map[string]string{common.ProxyCredentialSecret: "proxy-auth"})
	assert.Assert(t, err != nil)
	assert.Assert(t, strings.Contains(err.Error(), "requires a nonempty PROXY_URL"))
}

func TestValidateCICDProxySecretAllowsPrintableCredentials(t *testing.T) {
	for _, password := range []string{"p@ss w0rd", "p%40ss", "p#ss", "päss🔒"} {
		t.Run(password, func(t *testing.T) {
			assert.NilError(t, ValidateCICDProxySecret(validCICDProxySecret([]byte(password)), "workspace"))
		})
	}
}

func TestValidateCICDProxySecretRejectsControlCharacters(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     []byte
	}{
		{"password newline", "password", []byte("p@ss\nword")},
		{"password NUL", "password", []byte{'p', 0, 'w'}},
		{"username tab", "username", []byte("proxy\tuser")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			secret := validCICDProxySecret([]byte("password"))
			secret.Data[tc.key] = tc.value
			err := ValidateCICDProxySecret(secret, "workspace")
			assert.ErrorContains(t, err, tc.key+" key contains a control character")
		})
	}
}

func TestCICDProxyNoProxy(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	workload := &v1.Workload{}
	v1.SetAnnotation(workload, v1.AdminControlPlaneAnnotation, "10.245.157.232")

	merged := CICDProxyNoProxy(workload, " pypi.org , files.pythonhosted.org ", []string{"localhost", ".example.com"})

	for _, required := range []string{
		"localhost", ".svc", ".cluster.local", "10.96.0.1", "10.245.157.232", "pypi.org", ".example.com"} {
		if !slices.Contains(merged, required) {
			t.Fatalf("expected %q in %v", required, merged)
		}
	}
	if got := slices.Index(merged, "localhost"); got != 0 {
		t.Fatalf("reserved entries must come first, got %v", merged)
	}
	seen := map[string]int{}
	for _, entry := range merged {
		seen[entry]++
		if seen[entry] > 1 {
			t.Fatalf("duplicate entry %q in %v", entry, merged)
		}
	}
}

func TestCICDProxyNoProxyWithoutControlPlane(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	merged := CICDProxyNoProxy(&v1.Workload{}, "", nil)
	if slices.Contains(merged, "") {
		t.Fatalf("empty entry leaked into %v", merged)
	}
	if !slices.Contains(merged, ".cluster.local") {
		t.Fatalf("reserved entries missing from %v", merged)
	}
}
