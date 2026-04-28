package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
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
	if applied != 8 {
		t.Fatalf("expected 8 migrations applied, got %d", applied)
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
	if len(statuses) != 8 {
		t.Fatalf("expected 8 migration statuses, got %d", len(statuses))
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

func TestAdvertisementTunnelMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newAdvertisementTestFixture(t, ctx, store)

	defaulted, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      org.ID,
		AccountID:           acct.ID,
		Name:                "defaulted",
		Capabilities:        CapabilitiesJSON{{Name: "x"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wg.ID},
	})
	if err != nil {
		t.Fatalf("create defaulted: %v", err)
	}
	if defaulted.TunnelMode != TunnelModeTCP {
		t.Fatalf("expected default tunnel mode tcp, got %q", defaulted.TunnelMode)
	}

	explicit, err := store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      org.ID,
		AccountID:           acct.ID,
		Name:                "explicit-http",
		Capabilities:        CapabilitiesJSON{{Name: "x"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wg.ID},
		TunnelMode:          TunnelModeHTTP,
	})
	if err != nil {
		t.Fatalf("create explicit: %v", err)
	}
	if explicit.TunnelMode != TunnelModeHTTP {
		t.Fatalf("expected explicit tunnel mode http, got %q", explicit.TunnelMode)
	}

	reloaded, err := store.Advertisements.GetByID(ctx, store.DB(), explicit.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reloaded.TunnelMode != TunnelModeHTTP {
		t.Fatalf("reloaded tunnel mode mismatch: %q", reloaded.TunnelMode)
	}

	reloaded.TunnelMode = TunnelModeUDP
	updated, err := store.Advertisements.Update(ctx, store.DB(), *reloaded)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.TunnelMode != TunnelModeUDP {
		t.Fatalf("updated tunnel mode mismatch: %q", updated.TunnelMode)
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

// --- session tests ---

// newSessionTestFixture provisions two accounts (provider and consumer) in
// separate organizations sharing an inter-org workgroup, an advertisement
// owned by the provider, and a provider environment (for the Tunnel FK).
func newSessionTestFixture(t *testing.T, ctx context.Context, store *Store) (providerOrg, consumerOrg *Organization, providerAcct, consumerAcct *Account, providerEnv *Environment, wg *Workgroup, ad *Advertisement) {
	t.Helper()

	providerOrg, err := store.Organizations.Create(ctx, store.DB(), Organization{Name: "provider-co"})
	if err != nil {
		t.Fatalf("create provider org: %v", err)
	}
	consumerOrg, err = store.Organizations.Create(ctx, store.DB(), Organization{Name: "consumer-co"})
	if err != nil {
		t.Fatalf("create consumer org: %v", err)
	}
	providerName := "provider"
	providerAcct, err = store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: providerOrg.ID,
		Email:          "provider@example.com",
		DisplayName:    &providerName,
		PasswordSalt:   "salt-p",
		PasswordHash:   "hash-p",
		AccountToken:   "provider-token",
		Role:           AccountRoleAdmin,
		Status:         AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create provider account: %v", err)
	}
	consumerName := "consumer"
	consumerAcct, err = store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: consumerOrg.ID,
		Email:          "consumer@example.com",
		DisplayName:    &consumerName,
		PasswordSalt:   "salt-c",
		PasswordHash:   "hash-c",
		AccountToken:   "consumer-token",
		Role:           AccountRoleAdmin,
		Status:         AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create consumer account: %v", err)
	}

	envDescription := "provider-env"
	providerEnv, err = store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID: providerOrg.ID,
		AccountID:      providerAcct.ID,
		Description:    &envDescription,
		ZitiIdentityID: "ziti-provider-01",
		State:          EnvironmentStateEnabled,
	})
	if err != nil {
		t.Fatalf("create provider env: %v", err)
	}

	wg, err = store.Workgroups.Create(ctx, store.DB(), Workgroup{
		OwnerOrganizationID: providerOrg.ID,
		Name:                "shared-channel",
		Scope:               WorkgroupScopeInterOrg,
		State:               WorkgroupStateActive,
	})
	if err != nil {
		t.Fatalf("create workgroup: %v", err)
	}
	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID: wg.ID, OrganizationID: providerOrg.ID, AccountID: providerAcct.ID, Role: WorkgroupMembershipRoleAdmin,
	}); err != nil {
		t.Fatalf("create provider membership: %v", err)
	}
	if _, err := store.WorkgroupMemberships.Create(ctx, store.DB(), WorkgroupMembership{
		WorkgroupID: wg.ID, OrganizationID: consumerOrg.ID, AccountID: consumerAcct.ID, Role: WorkgroupMembershipRoleMember,
	}); err != nil {
		t.Fatalf("create consumer membership: %v", err)
	}

	ad, err = store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      providerOrg.ID,
		AccountID:           providerAcct.ID,
		Name:                "data-feed",
		Capabilities:        CapabilitiesJSON{{Name: "quote"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wg.ID},
		TunnelMode:          TunnelModeTCP,
	})
	if err != nil {
		t.Fatalf("create advertisement: %v", err)
	}
	return
}

func mustProposeSession(t *testing.T, ctx context.Context, store *Store, providerOrg, consumerOrg *Organization, providerAcct, consumerAcct *Account, wg *Workgroup, ad *Advertisement) *Session {
	t.Helper()
	sess, err := store.Sessions.Create(ctx, store.DB(), Session{
		AdvertisementID:        ad.ID,
		WorkgroupID:            wg.ID,
		ProviderAccountID:      providerAcct.ID,
		ProviderOrganizationID: providerOrg.ID,
		ConsumerAccountID:      consumerAcct.ID,
		ConsumerOrganizationID: consumerOrg.ID,
		TunnelMode:             ad.TunnelMode,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return sess
}

func TestSessionProposeDefaults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	providerOrg, consumerOrg, providerAcct, consumerAcct, _, wg, ad := newSessionTestFixture(t, ctx, store)

	sess := mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)

	if sess.State != SessionStateProposed {
		t.Fatalf("expected proposed, got %q", sess.State)
	}
	if sess.TunnelMode != TunnelModeTCP {
		t.Fatalf("expected tcp, got %q", sess.TunnelMode)
	}
	if sess.TunnelID != nil {
		t.Fatalf("expected nil tunnel id, got %v", sess.TunnelID)
	}
	if sess.AcceptedAt != nil || sess.ClosedAt != nil {
		t.Fatalf("unexpected accepted/closed timestamps")
	}
	if sess.ProposedAt.IsZero() {
		t.Fatalf("expected proposed_at populated")
	}
	if !strings.HasPrefix(sess.ID, "ses_") {
		t.Fatalf("id must have ses_ prefix: %q", sess.ID)
	}
}

func TestSessionAcceptFlow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	providerOrg, consumerOrg, providerAcct, consumerAcct, providerEnv, wg, ad := newSessionTestFixture(t, ctx, store)

	sess := mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)

	accepting, err := store.Sessions.MarkAccepting(ctx, store.DB(), sess.ID)
	if err != nil {
		t.Fatalf("mark accepting: %v", err)
	}
	if accepting.State != SessionStateAccepting {
		t.Fatalf("expected accepting, got %q", accepting.State)
	}

	zitiSvc := "ziti-svc-session"
	bindPol := "bind-session"
	tunnel, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: providerOrg.ID,
		AccountID:      providerAcct.ID,
		EnvironmentID:  providerEnv.ID,
		Name:           "session-tun",
		Mode:           TunnelModeTCP,
		BackendTarget:  "localhost:9000",
		ZitiServiceID:  &zitiSvc,
		BindPolicyID:   &bindPol,
		State:          TunnelStateActive,
	})
	if err != nil {
		t.Fatalf("create tunnel: %v", err)
	}

	active, err := store.Sessions.MarkActive(ctx, store.DB(), sess.ID, tunnel.ID)
	if err != nil {
		t.Fatalf("mark active: %v", err)
	}
	if active.State != SessionStateActive {
		t.Fatalf("expected active, got %q", active.State)
	}
	if active.TunnelID == nil || *active.TunnelID != tunnel.ID {
		t.Fatalf("tunnel id mismatch: %v", active.TunnelID)
	}
	if active.AcceptedAt == nil {
		t.Fatalf("expected accepted_at populated")
	}
}

func TestSessionRejectDirect(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	providerOrg, consumerOrg, providerAcct, consumerAcct, _, wg, ad := newSessionTestFixture(t, ctx, store)

	sess := mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)

	closed, err := store.Sessions.MarkRejected(ctx, store.DB(), sess.ID, "not taking new")
	if err != nil {
		t.Fatalf("mark rejected: %v", err)
	}
	if closed.State != SessionStateClosed {
		t.Fatalf("expected closed, got %q", closed.State)
	}
	if closed.CloseReason == nil || *closed.CloseReason != SessionCloseReasonRejected {
		t.Fatalf("close reason mismatch: %v", closed.CloseReason)
	}
	if closed.CloseDetail == nil || *closed.CloseDetail != "not taking new" {
		t.Fatalf("close detail mismatch: %v", closed.CloseDetail)
	}
}

func TestSessionCloseIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	providerOrg, consumerOrg, providerAcct, consumerAcct, _, wg, ad := newSessionTestFixture(t, ctx, store)

	sess := mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)
	if _, err := store.Sessions.MarkClosed(ctx, store.DB(), sess.ID, SessionCloseReasonConsumerClose, ""); err != nil {
		t.Fatalf("first close: %v", err)
	}
	again, err := store.Sessions.MarkClosed(ctx, store.DB(), sess.ID, SessionCloseReasonConsumerClose, "")
	if err != nil {
		t.Fatalf("idempotent close: %v", err)
	}
	if again.State != SessionStateClosed {
		t.Fatalf("expected closed after re-close, got %q", again.State)
	}
}

func TestSessionListFilters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	providerOrg, consumerOrg, providerAcct, consumerAcct, _, wg, ad := newSessionTestFixture(t, ctx, store)

	// three sessions
	a := mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)
	_ = mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)
	c := mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)

	if _, err := store.Sessions.MarkClosed(ctx, store.DB(), c.ID, SessionCloseReasonConsumerClose, ""); err != nil {
		t.Fatalf("close c: %v", err)
	}

	// as provider, see all three regardless of state
	all, err := store.Sessions.List(ctx, store.DB(), SessionListParams{
		ParticipantAccountID: providerAcct.ID, RoleFilter: "provider",
	})
	if err != nil {
		t.Fatalf("list provider: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}

	// filter to proposed only
	proposedOnly, err := store.Sessions.List(ctx, store.DB(), SessionListParams{
		ParticipantAccountID: providerAcct.ID, RoleFilter: "provider",
		States: []SessionState{SessionStateProposed},
	})
	if err != nil {
		t.Fatalf("list proposed: %v", err)
	}
	if len(proposedOnly) != 2 {
		t.Fatalf("expected 2 proposed, got %d", len(proposedOnly))
	}

	// as consumer, see all three
	asConsumer, err := store.Sessions.List(ctx, store.DB(), SessionListParams{
		ParticipantAccountID: consumerAcct.ID, RoleFilter: "consumer",
	})
	if err != nil {
		t.Fatalf("list consumer: %v", err)
	}
	if len(asConsumer) != 3 {
		t.Fatalf("expected 3 for consumer, got %d", len(asConsumer))
	}

	_ = a
}

// --- contract tests ---

func newContractTestFixture(t *testing.T, ctx context.Context, store *Store) (*Organization, *Account, *Workgroup) {
	t.Helper()
	return newAdvertisementTestFixture(t, ctx, store)
}

func TestContractCreateAndNameUniqueness(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newContractTestFixture(t, ctx, store)

	first, err := store.Contracts.Create(ctx, store.DB(), Contract{
		OrganizationID:               org.ID,
		AccountID:                    acct.ID,
		Name:                         "morning-brief",
		MaxDurationSeconds:           60,
		RequiredWorkgroupMemberships: pq.StringArray{wg.ID},
		AccessMode:                   ContractAccessModeApprovalRequired,
	})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	if first.ID == "" || !strings.HasPrefix(first.ID, "con_") {
		t.Fatalf("unexpected contract id: %q", first.ID)
	}
	if first.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", first.SchemaVersion)
	}

	if _, err := store.Contracts.Create(ctx, store.DB(), Contract{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Name:           "Morning-Brief",
		AccessMode:     ContractAccessModeApprovalRequired,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected unique violation on case-insensitive name collision, got %v", err)
	}
}

func TestContractGetByAccountAndName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, _ := newContractTestFixture(t, ctx, store)

	created, err := store.Contracts.Create(ctx, store.DB(), Contract{
		OrganizationID: org.ID, AccountID: acct.ID,
		Name: "by-name", AccessMode: ContractAccessModeOpen,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.Contracts.GetByAccountAndName(ctx, store.DB(), acct.ID, "BY-NAME")
	if err != nil {
		t.Fatalf("get by name: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected %s, got %s", created.ID, got.ID)
	}
	if _, err := store.Contracts.GetByAccountAndName(ctx, store.DB(), acct.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestContractMaturityJSONBRoundtrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, _ := newContractTestFixture(t, ctx, store)

	created, err := store.Contracts.Create(ctx, store.DB(), Contract{
		OrganizationID: org.ID, AccountID: acct.ID,
		Name:                 "mature",
		MaturityRequirements: MaturityRequirementsJSON{MinAccountAgeDays: 30},
		AccessMode:           ContractAccessModeOpen,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	loaded, err := store.Contracts.GetByID(ctx, store.DB(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if loaded.MaturityRequirements.MinAccountAgeDays != 30 {
		t.Fatalf("maturity did not round-trip: got %+v", loaded.MaturityRequirements)
	}
}

func TestContractDeleteReferenceBlocked(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newContractTestFixture(t, ctx, store)

	c, err := store.Contracts.Create(ctx, store.DB(), Contract{
		OrganizationID: org.ID, AccountID: acct.ID,
		Name: "ref", AccessMode: ContractAccessModeOpen,
	})
	if err != nil {
		t.Fatalf("create contract: %v", err)
	}
	_, err = store.Advertisements.Create(ctx, store.DB(), Advertisement{
		OrganizationID:      org.ID,
		AccountID:           acct.ID,
		Name:                "uses-contract",
		Capabilities:        CapabilitiesJSON{{Name: "x"}},
		InteractionPatterns: InteractionPatternsJSON{{Kind: InteractionPatternKindRequestResponse}},
		WorkgroupScopes:     []string{wg.ID},
		ContractID:          &c.ID,
	})
	if err != nil {
		t.Fatalf("create ad: %v", err)
	}

	inUse, err := store.Contracts.IsReferencedByActiveAdvertisement(ctx, store.DB(), c.ID)
	if err != nil {
		t.Fatalf("in-use check: %v", err)
	}
	if !inUse {
		t.Fatalf("expected in-use, got false")
	}
}

func TestContractUpdate(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, wg := newContractTestFixture(t, ctx, store)

	c, err := store.Contracts.Create(ctx, store.DB(), Contract{
		OrganizationID: org.ID, AccountID: acct.ID,
		Name: "editable", AccessMode: ContractAccessModeApprovalRequired,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	c.MaxDurationSeconds = 120
	c.MaxEnvelopeCount = 500
	c.AllowedMessageTypes = pq.StringArray{"markets.equity.request", "markets.equity.response"}
	c.RequiredWorkgroupMemberships = pq.StringArray{wg.ID}
	updated, err := store.Contracts.Update(ctx, store.DB(), *c)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.MaxDurationSeconds != 120 || updated.MaxEnvelopeCount != 500 {
		t.Fatalf("caps did not round-trip: %+v", updated)
	}
	if len(updated.AllowedMessageTypes) != 2 {
		t.Fatalf("allowed_message_types did not round-trip: %+v", updated.AllowedMessageTypes)
	}
}

func TestContractMaxEnvelopeBytesRoundtrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	org, acct, _ := newContractTestFixture(t, ctx, store)

	created, err := store.Contracts.Create(ctx, store.DB(), Contract{
		OrganizationID:   org.ID,
		AccountID:        acct.ID,
		Name:             "sized",
		MaxEnvelopeBytes: 512,
		AccessMode:       ContractAccessModeOpen,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.MaxEnvelopeBytes != 512 {
		t.Fatalf("expected 512, got %d", created.MaxEnvelopeBytes)
	}

	created.MaxEnvelopeBytes = 4096
	updated, err := store.Contracts.Update(ctx, store.DB(), *created)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.MaxEnvelopeBytes != 4096 {
		t.Fatalf("updated mtu mismatch: %d", updated.MaxEnvelopeBytes)
	}

	reloaded, err := store.Contracts.GetByID(ctx, store.DB(), created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reloaded.MaxEnvelopeBytes != 4096 {
		t.Fatalf("reloaded mtu mismatch: %d", reloaded.MaxEnvelopeBytes)
	}
}

func TestSessionRecordEnvelopeCountMaxSeen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := migratedTestStore(t)
	providerOrg, consumerOrg, providerAcct, consumerAcct, _, wg, ad := newSessionTestFixture(t, ctx, store)

	sess := mustProposeSession(t, ctx, store, providerOrg, consumerOrg, providerAcct, consumerAcct, wg, ad)

	if err := store.Sessions.RecordEnvelopeCount(ctx, store.DB(), sess.ID, 5); err != nil {
		t.Fatalf("record 5: %v", err)
	}
	reloaded, err := store.Sessions.GetByID(ctx, store.DB(), sess.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if reloaded.EnvelopeCount == nil || *reloaded.EnvelopeCount != 5 {
		t.Fatalf("expected 5, got %v", reloaded.EnvelopeCount)
	}

	// stale lower report is ignored (max-seen semantics).
	if err := store.Sessions.RecordEnvelopeCount(ctx, store.DB(), sess.ID, 3); err != nil {
		t.Fatalf("record 3: %v", err)
	}
	reloaded2, err := store.Sessions.GetByID(ctx, store.DB(), sess.ID)
	if err != nil {
		t.Fatalf("get 2: %v", err)
	}
	if reloaded2.EnvelopeCount == nil || *reloaded2.EnvelopeCount != 5 {
		t.Fatalf("expected count to stay at 5, got %v", reloaded2.EnvelopeCount)
	}

	// higher report advances.
	if err := store.Sessions.RecordEnvelopeCount(ctx, store.DB(), sess.ID, 9); err != nil {
		t.Fatalf("record 9: %v", err)
	}
	reloaded3, _ := store.Sessions.GetByID(ctx, store.DB(), sess.ID)
	if reloaded3.EnvelopeCount == nil || *reloaded3.EnvelopeCount != 9 {
		t.Fatalf("expected 9, got %v", reloaded3.EnvelopeCount)
	}
}
