package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/openziti/agora/internal/persistence/testutil"
	migrate "github.com/rubenv/sql-migrate"
)

func TestMigrateUpAndCompatibilityCurrent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t)

	applied, err := MigrateUp(ctx, store)
	if err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	if applied != 4 {
		t.Fatalf("expected 4 migrations applied, got %d", applied)
	}

	if err := CheckSchemaCompatibility(ctx, store); err != nil {
		t.Fatalf("check schema compatibility: %v", err)
	}
}

func TestCheckSchemaCompatibilityBehindBinary(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t)

	applied, err := execMigrations(ctx, store, migrate.Up, 1)
	if err != nil {
		t.Fatalf("apply single migration: %v", err)
	}
	if applied != 1 {
		t.Fatalf("expected 1 migration applied, got %d", applied)
	}

	statuses, err := MigrationStatusList(ctx, store)
	if err != nil {
		t.Fatalf("migration status: %v", err)
	}
	if len(statuses) != 4 {
		t.Fatalf("expected 4 migration statuses, got %d", len(statuses))
	}

	if err := CheckSchemaCompatibility(ctx, store); !errors.Is(err, ErrSchemaBehindBinary) {
		t.Fatalf("expected schema behind binary error, got %v", err)
	}
}

func TestCheckSchemaCompatibilityMissingMigrationsTable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := newTestStore(t)

	if err := CheckSchemaCompatibility(ctx, store); !errors.Is(err, ErrNoMigrationsTable) {
		t.Fatalf("expected missing migrations table error, got %v", err)
	}
}

func TestRepositoriesCRUDAndConstraints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)

	org, acct, env := createOrgAccountEnvironment(t, ctx, store)

	tunnelName := "llm-gateway"
	serviceID := "svc-123"
	tunnel, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		Name:           tunnelName,
		Mode:           TunnelModeTCP,
		BackendTarget:  "127.0.0.1:8443",
		ZitiServiceID:  &serviceID,
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	if _, err := store.Organizations.GetByID(ctx, store.DB(), org.ID); err != nil {
		t.Fatalf("get organization: %v", err)
	}
	if _, err := store.Accounts.GetByID(ctx, store.DB(), acct.ID); err != nil {
		t.Fatalf("get account: %v", err)
	}
	if _, err := store.Environments.GetByID(ctx, store.DB(), env.ID); err != nil {
		t.Fatalf("get environment: %v", err)
	}
	if _, err := store.Tunnels.GetByID(ctx, store.DB(), tunnel.ID); err != nil {
		t.Fatalf("get tunnel: %v", err)
	}

	lastSeen := time.Now().UTC().Truncate(time.Second)
	if err := store.Environments.UpdateLastSeen(ctx, store.DB(), env.ID, lastSeen); err != nil {
		t.Fatalf("update environment last seen: %v", err)
	}

	updatedEnv, err := store.Environments.GetByID(ctx, store.DB(), env.ID)
	if err != nil {
		t.Fatalf("get updated environment: %v", err)
	}
	if updatedEnv.LastSeenAt == nil || !updatedEnv.LastSeenAt.Truncate(time.Second).Equal(lastSeen) {
		t.Fatalf("expected last seen %v, got %v", lastSeen, updatedEnv.LastSeenAt)
	}

	if err := store.Environments.UpdateLastSeen(ctx, store.DB(), "ev_missing0000", lastSeen); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound updating missing environment, got %v", err)
	}

	if _, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org.ID,
		Email:          acct.Email,
		PasswordSalt:   "salt",
		PasswordHash:   "hash",
		AccountToken:   "dup-token",
		Role:           AccountRoleMember,
		Status:         AccountStatusActive,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected account unique violation, got %v", err)
	}

	if _, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		Name:           tunnelName,
		Mode:           TunnelModeTCP,
		BackendTarget:  "127.0.0.1:9443",
		State:          TunnelStateActive,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected tunnel unique violation, got %v", err)
	}
	if _, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		Name:           strings.ToUpper(tunnelName),
		Mode:           TunnelModeTCP,
		BackendTarget:  "127.0.0.1:10443",
		State:          TunnelStateActive,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected case-insensitive tunnel unique violation, got %v", err)
	}

	if _, err := store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID: org.ID,
		AccountID:      "ac_doesntexist0",
		ZitiIdentityID: "ziti-missing-account",
		State:          EnvironmentStateEnabled,
	}); !isForeignKeyViolation(err) {
		t.Fatalf("expected environment foreign key violation, got %v", err)
	}

	otherOrg, err := store.Organizations.Create(ctx, store.DB(), Organization{Name: "other-org"})
	if err != nil {
		t.Fatalf("create other organization: %v", err)
	}
	if _, err := store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID: otherOrg.ID,
		AccountID:      acct.ID,
		ZitiIdentityID: "ziti-cross-org",
		State:          EnvironmentStateEnabled,
	}); !isForeignKeyViolation(err) {
		t.Fatalf("expected cross-org environment foreign key violation, got %v", err)
	}
}

func TestWithTxRollsBackOnFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)

	orgID := NewResourceID(PrefixOrganization)
	err := store.WithTx(ctx, func(tx Queryer) error {
		_, err := store.Organizations.Create(ctx, tx, Organization{
			ID:   orgID,
			Name: "rollback-org",
		})
		if err != nil {
			return err
		}
		return errors.New("force rollback")
	})
	if err == nil {
		t.Fatal("expected transaction error")
	}

	if _, err := store.Organizations.GetByID(ctx, store.DB(), orgID); err == nil {
		t.Fatal("expected organization insert to be rolled back")
	}
}

func TestDeleteRepositories(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)

	org, acct, _ := createOrgAccountEnvironment(t, ctx, store)

	if err := store.Accounts.Delete(ctx, store.DB(), org.ID, acct.ID); err != nil {
		t.Fatalf("delete account: %v", err)
	}
	if _, err := store.Accounts.GetByID(ctx, store.DB(), acct.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected account to be deleted, got %v", err)
	}

	if err := store.Organizations.Delete(ctx, store.DB(), org.ID); err != nil {
		t.Fatalf("delete organization: %v", err)
	}
	if _, err := store.Organizations.GetByID(ctx, store.DB(), org.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected organization to be deleted, got %v", err)
	}

	org2, acct2, _ := createOrgAccountEnvironment(t, ctx, store)
	if err := store.Organizations.Delete(ctx, store.DB(), org2.ID); err != nil {
		t.Fatalf("delete organization with cascade: %v", err)
	}
	if _, err := store.Accounts.GetByID(ctx, store.DB(), acct2.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected cascaded account delete, got %v", err)
	}
}

func TestEnvironmentAndTunnelSoftDeleteBehavior(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)

	org, acct, env := createOrgAccountEnvironment(t, ctx, store)
	tunnel, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		Name:           "llm-gateway",
		Mode:           TunnelModeTCP,
		BackendTarget:  "127.0.0.1:8443",
		State:          TunnelStateActive,
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	if err := store.Tunnels.Delete(ctx, store.DB(), tunnel.ID, org.ID); err != nil {
		t.Fatalf("delete tunnel: %v", err)
	}
	if _, err := store.Tunnels.GetByID(ctx, store.DB(), tunnel.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted tunnel to be hidden, got %v", err)
	}
	if _, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		EnvironmentID:  env.ID,
		Name:           "llm-gateway",
		Mode:           TunnelModeTCP,
		BackendTarget:  "127.0.0.1:9443",
		State:          TunnelStateActive,
	}); err != nil {
		t.Fatalf("expected tunnel name reuse after soft delete, got %v", err)
	}

	if err := store.Environments.Delete(ctx, store.DB(), env.ID, org.ID); err != nil {
		t.Fatalf("delete environment: %v", err)
	}
	if _, err := store.Environments.GetByID(ctx, store.DB(), env.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected deleted environment to be hidden, got %v", err)
	}
	environments, err := store.Environments.ListByOrganization(ctx, store.DB(), org.ID)
	if err != nil {
		t.Fatalf("list environments: %v", err)
	}
	if len(environments) != 0 {
		t.Fatalf("expected 0 active environments, got %d", len(environments))
	}
}

func TestAccountsListSupportsGlobalAndFilteredQueries(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)

	org1, _, _ := createOrgAccountEnvironment(t, ctx, store)
	org2, err := store.Organizations.Create(ctx, store.DB(), Organization{Name: "beta"})
	if err != nil {
		t.Fatalf("create second organization: %v", err)
	}
	if _, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org2.ID,
		Email:          "bob@example.com",
		PasswordSalt:   "salt-2",
		PasswordHash:   "hash-2",
		AccountToken:   "account-token-2",
		Role:           AccountRoleMember,
		Status:         AccountStatusActive,
	}); err != nil {
		t.Fatalf("create second account: %v", err)
	}

	allAccounts, err := store.Accounts.List(ctx, store.DB(), nil)
	if err != nil {
		t.Fatalf("list all accounts: %v", err)
	}
	if len(allAccounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(allAccounts))
	}

	filteredAccounts, err := store.Accounts.List(ctx, store.DB(), &org1.ID)
	if err != nil {
		t.Fatalf("list filtered accounts: %v", err)
	}
	if len(filteredAccounts) != 1 {
		t.Fatalf("expected 1 filtered account, got %d", len(filteredAccounts))
	}
	if filteredAccounts[0].OrganizationID != org1.ID {
		t.Fatalf("expected filtered account in organization %s, got %s", org1.ID, filteredAccounts[0].OrganizationID)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	ctx := context.Background()
	store, err := Open(ctx, Config{
		DSN:             testutil.StartPostgres(t),
		MaxOpenConns:    4,
		MaxIdleConns:    4,
		ConnMaxLifetime: time.Hour,
	})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	return store
}

func migratedTestStore(t *testing.T) *Store {
	t.Helper()

	ctx := context.Background()
	store := newTestStore(t)
	if _, err := MigrateUp(ctx, store); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
	return store
}

func createOrgAccountEnvironment(t *testing.T, ctx context.Context, store *Store) (*Organization, *Account, *Environment) {
	t.Helper()

	org, err := store.Organizations.Create(ctx, store.DB(), Organization{Name: "acme"})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	displayName := "Alice Admin"
	acct, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org.ID,
		Email:          "alice@example.com",
		DisplayName:    &displayName,
		PasswordSalt:   "salt-1",
		PasswordHash:   "hash-1",
		AccountToken:   "account-token-1",
		Role:           AccountRoleAdmin,
		Status:         AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	description := "build agent"
	host := "agent-01"
	edgeRouterPolicyID := "erp-01"
	env, err := store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID:     org.ID,
		AccountID:          acct.ID,
		Description:        &description,
		Host:               &host,
		ZitiIdentityID:     "ziti-identity-01",
		EdgeRouterPolicyID: &edgeRouterPolicyID,
		State:              EnvironmentStateEnabled,
	})
	if err != nil {
		t.Fatalf("create environment: %v", err)
	}

	return org, acct, env
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func TestWorkgroupNameUniquenessWithinOrg(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	org, _, _ := createOrgAccountEnvironment(t, ctx, store)

	first, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "Research-Team",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create first workgroup: %v", err)
	}

	if _, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "research-team",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected unique violation on case-insensitive name reuse, got %v", err)
	}

	if first.State != WorkgroupStateActive {
		t.Fatalf("expected created workgroup to be active, got %v", first.State)
	}
}

func TestWorkgroupNameReuseAfterDeclined(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	org, _, _ := createOrgAccountEnvironment(t, ctx, store)

	first, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "Pilot",
		Scope:               WorkgroupScopeInterOrg,
		State:               WorkgroupStatePending,
	})
	if err != nil {
		t.Fatalf("create first workgroup: %v", err)
	}
	if err := store.Workgroups.UpdateState(ctx, store.DB(), first.ID, WorkgroupStateDeclined); err != nil {
		t.Fatalf("transition to declined: %v", err)
	}

	if _, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "Pilot",
		Scope:               WorkgroupScopeInterOrg,
		State:               WorkgroupStatePending,
	}); err != nil {
		t.Fatalf("expected name reuse after declined to succeed, got %v", err)
	}
}

func TestWorkgroupInvitationUniquenessAndStateTransitions(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	orgA, _, _ := createOrgAccountEnvironment(t, ctx, store)

	orgB, err := store.Organizations.Create(ctx, store.DB(), Organization{Name: "org-b"})
	if err != nil {
		t.Fatalf("create org-b: %v", err)
	}

	wg, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: orgA.ID,
		Name:                "shared-channel",
		Scope:               WorkgroupScopeInterOrg,
		State:               WorkgroupStatePending,
	})
	if err != nil {
		t.Fatalf("create workgroup: %v", err)
	}

	if _, err := store.WorkgroupInvitations.Create(ctx, store.DB(), WorkgroupInvitation{
		WorkgroupID:    wg.ID,
		OrganizationID: orgB.ID,
		State:          WorkgroupInvitationStatePending,
	}); err != nil {
		t.Fatalf("create invitation: %v", err)
	}

	if _, err := store.WorkgroupInvitations.Create(ctx, store.DB(), WorkgroupInvitation{
		WorkgroupID:    wg.ID,
		OrganizationID: orgB.ID,
		State:          WorkgroupInvitationStatePending,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected unique violation on duplicate invitation, got %v", err)
	}

	invitation, err := store.WorkgroupInvitations.GetByWorkgroupAndOrg(ctx, store.DB(), wg.ID, orgB.ID)
	if err != nil {
		t.Fatalf("get invitation: %v", err)
	}
	acknowledgedBy := "admin"
	acknowledgedAt := time.Now().UTC()
	if err := store.WorkgroupInvitations.UpdateState(ctx, store.DB(), invitation.ID, WorkgroupInvitationStateAccepted, acknowledgedBy, acknowledgedAt); err != nil {
		t.Fatalf("update invitation state: %v", err)
	}

	updated, err := store.WorkgroupInvitations.GetByID(ctx, store.DB(), invitation.ID)
	if err != nil {
		t.Fatalf("get updated invitation: %v", err)
	}
	if updated.State != WorkgroupInvitationStateAccepted {
		t.Fatalf("expected accepted state, got %v", updated.State)
	}
	if updated.AcknowledgedByAccountID == nil || *updated.AcknowledgedByAccountID != acknowledgedBy {
		t.Fatalf("expected acknowledged_by_account_id to be %q, got %v", acknowledgedBy, updated.AcknowledgedByAccountID)
	}
	if updated.AcknowledgedAt == nil {
		t.Fatalf("expected acknowledged_at to be set")
	}
}

func TestWorkgroupMembershipUniquenessAndCountAdmins(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, _ := createOrgAccountEnvironment(t, ctx, store)

	wg, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "team",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create workgroup: %v", err)
	}

	first, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    wg.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Role:           WorkgroupMembershipRoleAdmin,
	})
	if err != nil {
		t.Fatalf("create first membership: %v", err)
	}

	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    wg.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Role:           WorkgroupMembershipRoleMember,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected unique violation on duplicate membership, got %v", err)
	}

	count, err := store.WorkgroupMemberships.CountAdmins(ctx, store.DB(), wg.ID)
	if err != nil {
		t.Fatalf("count admins: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 admin, got %d", count)
	}

	if err := store.WorkgroupMemberships.MarkDeleted(ctx, store.DB(), first.ID); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}

	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    wg.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Role:           WorkgroupMembershipRoleMember,
	}); err != nil {
		t.Fatalf("expected re-create of soft-deleted membership to succeed, got %v", err)
	}

	count, err = store.WorkgroupMemberships.CountAdmins(ctx, store.DB(), wg.ID)
	if err != nil {
		t.Fatalf("count admins after re-create: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 admins after deletion + member-only re-create, got %d", count)
	}
}

func TestWorkgroupListAccessibleByAccountFiltersByMembership(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, _ := createOrgAccountEnvironment(t, ctx, store)

	wgIn, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "joined",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create joined workgroup: %v", err)
	}
	if _, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "other",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	}); err != nil {
		t.Fatalf("create other workgroup: %v", err)
	}

	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    wgIn.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Role:           WorkgroupMembershipRoleMember,
	}); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	visible, err := store.Workgroups.ListAccessibleByAccount(ctx, store.DB(), acct.ID)
	if err != nil {
		t.Fatalf("list accessible: %v", err)
	}
	if len(visible) != 1 || visible[0].ID != wgIn.ID {
		ids := make([]string, len(visible))
		for i, w := range visible {
			ids[i] = w.ID
		}
		t.Fatalf("expected only joined workgroup, got %v", strings.Join(ids, ", "))
	}
}

func TestWorkgroupIDFormatConstraint(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := migratedTestStore(t)
	org, _, _ := createOrgAccountEnvironment(t, ctx, store)

	if _, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		ID:                  "wg_INVALID",
		OwnerOrganizationID: org.ID,
		Name:                "bad-id",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	}); err == nil {
		t.Fatalf("expected error on invalid wg id, got nil")
	}
}

func newAdvertisementTestFixture(t *testing.T, ctx context.Context, store *Store) (*Organization, *Account, *Workgroup) {
	t.Helper()
	org, acct, _ := createOrgAccountEnvironment(t, ctx, store)
	wg, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "ad-test-wg",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create workgroup: %v", err)
	}
	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    wg.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Role:           WorkgroupMembershipRoleAdmin,
	}); err != nil {
		t.Fatalf("create membership: %v", err)
	}
	return org, acct, wg
}

func TestAdvertisementNameUniquenessWithinAccount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newAdvertisementTestFixture(t, ctx, store)

	first, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:  org.ID,
		AccountID:       acct.ID,
		Name:            "Sum-Service",
		WorkgroupScopes: []string{wg.ID},
		Capabilities:    CapabilitiesJSON{{Name: "summarize"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
	})
	if err != nil {
		t.Fatalf("create first advertisement: %v", err)
	}

	if _, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:  org.ID,
		AccountID:       acct.ID,
		Name:            "sum-service",
		WorkgroupScopes: []string{wg.ID},
		Capabilities:    CapabilitiesJSON{{Name: "summarize"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
	}); !isUniqueViolation(err) {
		t.Fatalf("expected unique violation on case-insensitive name reuse, got %v", err)
	}

	if err := store.Advertisements.MarkRetracted(ctx, store.DB(), first.ID); err != nil {
		t.Fatalf("retract: %v", err)
	}

	if _, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:  org.ID,
		AccountID:       acct.ID,
		Name:            "Sum-Service",
		WorkgroupScopes: []string{wg.ID},
		Capabilities:    CapabilitiesJSON{{Name: "summarize"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
	}); err != nil {
		t.Fatalf("expected name reuse after retract to succeed, got %v", err)
	}
}

func TestAdvertisementJSONBRoundtrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newAdvertisementTestFixture(t, ctx, store)

	caps := CapabilitiesJSON{
		{Name: "summarize", Description: "summarize a body of text", Metadata: map[string]string{"a2a_kind": "tool", "version": "v1"}},
		{Name: "translate", Description: "convert text to a target language"},
	}
	patterns := InteractionPatternsJSON{
		{Kind: InteractionPatternKindRequestResponse},
		{Kind: InteractionPatternKindCustom, CustomPattern: "websocket-stream"},
	}

	created, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      org.ID,
		AccountID:           acct.ID,
		Name:                "rich-ad",
		Capabilities:        caps,
		InteractionPatterns: patterns,
		WorkgroupScopes:     []string{wg.ID},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	loaded, err := store.Advertisements.GetByID(ctx, store.DB(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(loaded.Capabilities) != 2 || loaded.Capabilities[0].Name != "summarize" {
		t.Fatalf("unexpected capabilities round-trip: %#v", loaded.Capabilities)
	}
	if loaded.Capabilities[0].Metadata["a2a_kind"] != "tool" {
		t.Fatalf("metadata did not round-trip: %#v", loaded.Capabilities[0].Metadata)
	}
	if len(loaded.InteractionPatterns) != 2 || loaded.InteractionPatterns[1].CustomPattern != "websocket-stream" {
		t.Fatalf("unexpected interaction patterns round-trip: %#v", loaded.InteractionPatterns)
	}
}

func TestAdvertisementWorkgroupScopesArray(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newAdvertisementTestFixture(t, ctx, store)

	wg2, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "second-wg",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create second workgroup: %v", err)
	}

	created, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      org.ID,
		AccountID:           acct.ID,
		Name:                "scoped-ad",
		WorkgroupScopes:     []string{wg.ID, wg2.ID},
		Capabilities:        CapabilitiesJSON{{Name: "x"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindStream}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	loaded, err := store.Advertisements.GetByID(ctx, store.DB(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(loaded.WorkgroupScopes) != 2 {
		t.Fatalf("expected 2 workgroup scopes, got %d", len(loaded.WorkgroupScopes))
	}
}

func TestAdvertisementSearchVisibility(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, ownerAcct, ownerWG := newAdvertisementTestFixture(t, ctx, store)

	// publish an ad scoped to ownerWG
	publishedAd, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      org.ID,
		AccountID:           ownerAcct.ID,
		Name:                "owner-ad",
		WorkgroupScopes:     []string{ownerWG.ID},
		Capabilities:        CapabilitiesJSON{{Name: "owned"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// caller A: member of ownerWG, NOT the owner
	memberAcct, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org.ID,
		Email:          "member@example.com",
		PasswordSalt:   "s", PasswordHash: "h", AccountToken: "tok-member",
		Role:   AccountRoleMember,
		Status: AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create member account: %v", err)
	}
	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    ownerWG.ID,
		OrganizationID: org.ID,
		AccountID:      memberAcct.ID,
		Role:           WorkgroupMembershipRoleMember,
	}); err != nil {
		t.Fatalf("create membership: %v", err)
	}

	// caller B: in same org, NOT a member of ownerWG
	outsiderAcct, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org.ID,
		Email:          "outsider@example.com",
		PasswordSalt:   "s", PasswordHash: "h", AccountToken: "tok-outsider",
		Role:   AccountRoleMember,
		Status: AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create outsider account: %v", err)
	}

	memberResults, _, err := store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:    memberAcct.ID,
		CallerWorkgroupIDs: []string{ownerWG.ID},
	})
	if err != nil {
		t.Fatalf("member search: %v", err)
	}
	if len(memberResults) != 1 || memberResults[0].ID != publishedAd.ID {
		t.Fatalf("expected member to see the ad, got %d results", len(memberResults))
	}

	outsiderResults, _, err := store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:    outsiderAcct.ID,
		CallerWorkgroupIDs: nil,
	})
	if err != nil {
		t.Fatalf("outsider search: %v", err)
	}
	if len(outsiderResults) != 0 {
		t.Fatalf("expected outsider to see no ads, got %d", len(outsiderResults))
	}

	ownerResults, _, err := store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:    ownerAcct.ID,
		CallerWorkgroupIDs: []string{ownerWG.ID},
	})
	if err != nil {
		t.Fatalf("owner search: %v", err)
	}
	if len(ownerResults) != 1 {
		t.Fatalf("expected owner to see own ad, got %d", len(ownerResults))
	}
}

func TestAdvertisementSearchPagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newAdvertisementTestFixture(t, ctx, store)

	for i := 0; i < 3; i++ {
		_, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
			OrganizationID:      org.ID,
			AccountID:           acct.ID,
			Name:                fmt.Sprintf("paged-%d", i),
			WorkgroupScopes:     []string{wg.ID},
			Capabilities:        CapabilitiesJSON{{Name: "x"}},
			InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
		})
		if err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		time.Sleep(time.Millisecond * 5) // ensure distinct timestamps for sort stability
	}

	page1, cursor1, err := store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:    acct.ID,
		CallerWorkgroupIDs: []string{wg.ID},
		Limit:              2,
	})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1) != 2 || cursor1 == "" {
		t.Fatalf("expected page 1 to have 2 items + cursor, got %d items / cursor=%q", len(page1), cursor1)
	}

	decoded, err := DecodeSearchCursor(cursor1)
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}

	page2, cursor2, err := store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:    acct.ID,
		CallerWorkgroupIDs: []string{wg.ID},
		Limit:              2,
		Cursor:             decoded,
	})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected page 2 to have 1 item, got %d", len(page2))
	}
	if cursor2 != "" {
		t.Fatalf("expected no cursor on final page, got %q", cursor2)
	}
}

func TestAdvertisementSearchFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newAdvertisementTestFixture(t, ctx, store)

	wg2, err := store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: org.ID,
		Name:                "wg-two",
		Scope:               WorkgroupScopeIntraOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create wg2: %v", err)
	}
	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID:    wg2.ID,
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Role:           WorkgroupMembershipRoleMember,
	}); err != nil {
		t.Fatalf("create wg2 membership: %v", err)
	}

	mustCreate := func(name string, caps CapabilitiesJSON, patterns InteractionPatternsJSON, scopes []string) {
		t.Helper()
		if _, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
			OrganizationID:      org.ID,
			AccountID:           acct.ID,
			Name:                name,
			WorkgroupScopes:     scopes,
			Capabilities:        caps,
			InteractionPatterns: patterns,
		}); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	mustCreate("translator", CapabilitiesJSON{{Name: "translate"}}, InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}}, []string{wg.ID})
	mustCreate("summary-bot", CapabilitiesJSON{{Name: "summarize"}}, InteractionPatternsJSON{{Kind: InteractionPatternKindStream}}, []string{wg.ID})
	mustCreate("wg2-only", CapabilitiesJSON{{Name: "summarize"}}, InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}}, []string{wg2.ID})

	callerScopes := []string{wg.ID, wg2.ID}

	// capability keyword
	results, _, err := store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:    acct.ID,
		CallerWorkgroupIDs: callerScopes,
		CapabilityKeyword:  "summar",
	})
	if err != nil {
		t.Fatalf("capability search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 summary-related ads, got %d", len(results))
	}

	// interaction pattern
	results, _, err = store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:         acct.ID,
		CallerWorkgroupIDs:      callerScopes,
		InteractionPatternKinds: []InteractionPatternKind{InteractionPatternKindStream},
	})
	if err != nil {
		t.Fatalf("interaction search: %v", err)
	}
	if len(results) != 1 || results[0].Name != "summary-bot" {
		t.Fatalf("expected summary-bot, got %d results", len(results))
	}

	// workgroup filter
	results, _, err = store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:    acct.ID,
		CallerWorkgroupIDs: callerScopes,
		WorkgroupFilter:    []string{wg2.ID},
	})
	if err != nil {
		t.Fatalf("workgroup filter search: %v", err)
	}
	if len(results) != 1 || results[0].Name != "wg2-only" {
		t.Fatalf("expected wg2-only, got %d results", len(results))
	}

	// owner-org
	results, _, err = store.Advertisements.Search(ctx, store.DB(), SearchParams{
		CallerAccountID:     acct.ID,
		CallerWorkgroupIDs:  callerScopes,
		OwnerOrganizationID: org.ID,
	})
	if err != nil {
		t.Fatalf("owner-org search: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 ads from this org, got %d", len(results))
	}
}
