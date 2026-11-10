package scim

import (
	"testing"

	v1 "github.com/AMD-AIG-AIMA/SAFE/apis/pkg/apis/amd/v1"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

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
