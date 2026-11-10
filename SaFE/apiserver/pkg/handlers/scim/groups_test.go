package scim

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// The apiserver decodes JSON strictly (server.go enables DisallowUnknownFields). A SCIM client sends the PatchOp
// schemas member, so the patch body must declare it.
func TestParsePatchAcceptsSCIMPatchOpSchemas(t *testing.T) {
	gin.SetMode(gin.TestMode)
	binding.EnableDecoderDisallowUnknownFields = true
	t.Cleanup(func() { binding.EnableDecoderDisallowUnknownFields = false })
	for _, tc := range []struct {
		name, body string
		groups     bool
	}{
		{"clear groups", `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"groups","value":[]}]}`, true},
		{"set groups", `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"groups","value":["system-admin"]}]}`, true},
		{"deactivate", `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"active","value":false}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPatch, "/scim/v2/Users/x", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/scim+json")
			plan, err := parsePatch(c)
			if err != nil {
				t.Fatalf("parsePatch: %v", err)
			}
			if (plan.groups != nil) != tc.groups {
				t.Fatalf("groups set %v, want %v", plan.groups != nil, tc.groups)
			}
		})
	}
}

func TestApplyMappedRolesReplacesPlatformRoles(t *testing.T) {
	viper.Set("auth.group_mappings", []map[string]any{
		{"group": "system-admin", "roles": []string{"system-admin"}},
		{"group": "system-admin-readonly", "roles": []string{"system-admin-readonly"}},
	})
	user := &v1.User{ObjectMeta: metav1.ObjectMeta{Name: "u1"}, Spec: v1.UserSpec{Roles: []v1.UserRole{"default", v1.SystemAdminReadonlyRole}}}
	changed, err := applyMappedRoles(user, []string{"system-admin"})
	if err != nil || !changed {
		t.Fatalf("changed %v err %v", changed, err)
	}
	if !v1.IsContainRole(user.Spec.Roles, "default") || !v1.IsContainRole(user.Spec.Roles, v1.SystemAdminRole) ||
		v1.IsContainRole(user.Spec.Roles, v1.SystemAdminReadonlyRole) {
		t.Fatalf("roles %v", user.Spec.Roles)
	}
	changed, err = applyMappedRoles(user, []string{})
	if err != nil || !changed {
		t.Fatalf("clear changed %v err %v", changed, err)
	}
	if !v1.IsContainRole(user.Spec.Roles, "default") || v1.IsContainRole(user.Spec.Roles, v1.SystemAdminRole) {
		t.Fatalf("roles after clear %v", user.Spec.Roles)
	}
}
