package persistence

import (
	"context"
	"errors"
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
	if applied != 2 {
		t.Fatalf("expected 2 migrations applied, got %d", applied)
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
	if len(statuses) != 2 {
		t.Fatalf("expected 2 migration statuses, got %d", len(statuses))
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
