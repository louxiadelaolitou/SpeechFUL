package authprivacyruntime

import (
	"context"
	"testing"

	"github.com/zhubaozhenshuai666-lang/HnuHole/services/api/internal/authprivacyhttp"
)

func TestPasswordLoginRuntimePrivilegesPostgres(t *testing.T) {
	owner, runtime := moderationStartupLab(t)
	ctx := context.Background()
	if err := CheckDatabase(ctx, runtime, authprivacyhttp.CommunityService); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"password_login_limits"} {
		for _, privilege := range []string{"SELECT", "INSERT", "UPDATE"} {
			if _, err := owner.Exec(ctx, "REVOKE "+privilege+" ON c_auth."+table+" FROM hnuhole_c_runtime"); err != nil {
				t.Fatal(err)
			}
			if err := CheckDatabase(ctx, runtime, authprivacyhttp.CommunityService); err == nil {
				t.Fatal("missing password login permission accepted", table, privilege)
			}
			if _, err := owner.Exec(ctx, "GRANT "+privilege+" ON c_auth."+table+" TO hnuhole_c_runtime"); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := owner.Exec(ctx, "DROP TABLE c_auth.password_login_limits"); err != nil {
		t.Fatal(err)
	}
	if err := CheckDatabase(ctx, runtime, authprivacyhttp.CommunityService); err == nil {
		t.Fatal("missing password login schema accepted")
	}
}
