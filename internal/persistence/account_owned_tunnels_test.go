package persistence

import (
	"context"
	"testing"
)

// a standalone (account-owned) tunnel carries a NULL environment_id and is not returned by
// ListByEnvironment; a session tunnel carries a set environment_id and is. the null/set split is the
// standalone-vs-session discriminator the retire handler turns on.
func TestAccountOwnedTunnelEnvironmentNullable(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, env := createOrgAccountEnvironment(t, ctx, store)

	standalone, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  nil,
		Name:           "standalone",
		Mode:           TunnelModeHTTP,
		Kind:           TunnelKindDirect,
		State:          TunnelStateActive,
	})
	if err != nil {
		t.Fatalf("create standalone tunnel: %v", err)
	}
	if standalone.EnvironmentID != nil {
		t.Fatalf("expected nil environment_id on standalone tunnel, got %q", *standalone.EnvironmentID)
	}

	reloaded, err := store.Tunnels.GetByID(ctx, store.DB(), standalone.ID)
	if err != nil {
		t.Fatalf("get standalone tunnel: %v", err)
	}
	if reloaded.EnvironmentID != nil {
		t.Fatalf("expected nil environment_id after reload, got %q", *reloaded.EnvironmentID)
	}

	session, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  stringPtr(env.ID),
		Name:           "session",
		Mode:           TunnelModeHTTP,
		Kind:           TunnelKindDirect,
		State:          TunnelStateActive,
	})
	if err != nil {
		t.Fatalf("create session tunnel: %v", err)
	}

	byEnv, err := store.Tunnels.ListByEnvironment(ctx, store.DB(), env.ID, org.ID)
	if err != nil {
		t.Fatalf("list by environment: %v", err)
	}
	if len(byEnv) != 1 || byEnv[0].ID != session.ID {
		t.Fatalf("expected ListByEnvironment to return only the session tunnel, got %d rows: %#v", len(byEnv), byEnv)
	}
}

// finding C1: the account-scoped env FK enforces org -> account -> environment in the schema. a tunnel
// that references an environment must reference one owned by the tunnel's own account.
func TestAccountScopedEnvFKRejectsForeignAccount(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)
	org, _, env := createOrgAccountEnvironment(t, ctx, store)

	other, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org.ID,
		Email:          "bob@example.com",
		PasswordSalt:   "salt-2",
		PasswordHash:   "hash-2",
		AccountToken:   "account-token-2",
		Role:           AccountRoleMember,
		Status:         AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create second account: %v", err)
	}

	// env is owned by the first account; referencing it from a tunnel owned by 'other' has no matching
	// (id, account_id, organization_id) row in environments, so the FK rejects it.
	_, err = store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      other.ID,
		EnvironmentID:  stringPtr(env.ID),
		Name:           "foreign-session",
		Mode:           TunnelModeHTTP,
		Kind:           TunnelKindDirect,
		State:          TunnelStateActive,
	})
	if !isForeignKeyViolation(err) {
		t.Fatalf("expected foreign-key violation creating a session tunnel against another account's environment, got %v", err)
	}
}

// the env FK is NO ACTION (not CASCADE): a hard environment-row delete no longer drags standalone
// tunnels with it (they have NULL environment_id and are exempt via MATCH SIMPLE), while a session
// tunnel that still references the environment blocks the delete. the retire handler relies on this by
// tearing down session tunnels before deleting the environment row.
func TestEnvHardDeleteForeignKeySemantics(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, _ := createOrgAccountEnvironment(t, ctx, store)

	newEnv := func(zitiID string) *Environment {
		env, err := store.Environments.Create(ctx, store.DB(), Environment{
			OrganizationID: org.ID,
			AccountID:      acct.ID,
			ZitiIdentityID: zitiID,
			State:          EnvironmentStateEnabled,
		})
		if err != nil {
			t.Fatalf("create environment %q: %v", zitiID, err)
		}
		return env
	}

	t.Run("standalone survives env hard delete", func(t *testing.T) {
		env := newEnv("ziti-identity-standalone")
		standalone, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
			OrganizationID: org.ID,
			AccountID:      acct.ID,
			EnvironmentID:  nil,
			Name:           "survivor",
			Mode:           TunnelModeHTTP,
			Kind:           TunnelKindDirect,
			State:          TunnelStateActive,
		})
		if err != nil {
			t.Fatalf("create standalone tunnel: %v", err)
		}

		if _, err := store.DB().ExecContext(ctx, "delete from environments where id = $1", env.ID); err != nil {
			t.Fatalf("hard delete environment: %v", err)
		}

		if _, err := store.Tunnels.GetByID(ctx, store.DB(), standalone.ID); err != nil {
			t.Fatalf("expected standalone tunnel to survive environment deletion, got %v", err)
		}
	})

	t.Run("session tunnel blocks env hard delete", func(t *testing.T) {
		env := newEnv("ziti-identity-session")
		if _, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
			OrganizationID: org.ID,
			AccountID:      acct.ID,
			EnvironmentID:  stringPtr(env.ID),
			Name:           "bound",
			Mode:           TunnelModeHTTP,
			Kind:           TunnelKindDirect,
			State:          TunnelStateActive,
		}); err != nil {
			t.Fatalf("create session tunnel: %v", err)
		}

		_, err := store.DB().ExecContext(ctx, "delete from environments where id = $1", env.ID)
		if !isForeignKeyViolation(err) {
			t.Fatalf("expected NO ACTION env FK to block deleting an environment a session tunnel references, got %v", err)
		}
	})
}

// finding C3: the NO ACTION env FK does not block tenant teardown. deleting an account hard-deletes its
// environments (account FK cascade) and its tunnels/serves/attachments (account/tunnel FK cascades) in
// one statement, so a session tunnel referencing the environment is removed alongside it rather than
// blocking the delete. a standalone tunnel cascades the same way via the account FK.
func TestTenantTeardownClearsTunnelsAndParticipation(t *testing.T) {
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, env := createOrgAccountEnvironment(t, ctx, store)

	session, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  stringPtr(env.ID),
		Name:           "session",
		Mode:           TunnelModeHTTP,
		Kind:           TunnelKindDirect,
		State:          TunnelStateActive,
	})
	if err != nil {
		t.Fatalf("create session tunnel: %v", err)
	}
	if _, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  nil,
		Name:           "standalone",
		Mode:           TunnelModeHTTP,
		Kind:           TunnelKindDirect,
		State:          TunnelStateActive,
	}); err != nil {
		t.Fatalf("create standalone tunnel: %v", err)
	}
	if _, err := store.TunnelServes.Create(ctx, store.DB(), TunnelServe{
		TunnelID:       session.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		State:          TunnelServeStateActive,
	}); err != nil {
		t.Fatalf("create serve: %v", err)
	}
	if _, err := store.TunnelAttachments.Create(ctx, store.DB(), TunnelAttachment{
		TunnelID:       session.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		ListenAddress:  stringPtr("127.0.0.1:0"),
		State:          TunnelAttachmentStateActive,
	}); err != nil {
		t.Fatalf("create attachment: %v", err)
	}

	if err := store.Accounts.Delete(ctx, store.DB(), org.ID, acct.ID); err != nil {
		t.Fatalf("expected account delete to succeed despite a session tunnel referencing the environment, got %v", err)
	}

	for _, tbl := range []string{"tunnels", "tunnel_serves", "tunnel_attachments", "environments"} {
		var count int
		if err := store.DB().GetContext(ctx, &count, "select count(1) from "+tbl+" where account_id = $1", acct.ID); err != nil {
			t.Fatalf("count %s: %v", tbl, err)
		}
		if count != 0 {
			t.Fatalf("expected no residual %s rows after account teardown, found %d", tbl, count)
		}
	}

	// the org is now empty and can be removed.
	if err := store.Organizations.Delete(ctx, store.DB(), org.ID); err != nil {
		t.Fatalf("expected empty organization delete to succeed, got %v", err)
	}
}
