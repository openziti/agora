package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
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
		EnvironmentID:  env.ID,
		Name:           tunnelName,
		BackendAddress: "127.0.0.1:8443",
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

	if _, err := store.Accounts.Create(ctx, store.DB(), Account{
		OrganizationID: org.ID,
		Email:          acct.Email,
		Role:           AccountRoleMember,
		Status:         AccountStatusActive,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected account unique violation, got %v", err)
	}

	if _, err := store.Tunnels.Create(ctx, store.DB(), Tunnel{
		OrganizationID: org.ID,
		EnvironmentID:  env.ID,
		Name:           tunnelName,
		BackendAddress: "127.0.0.1:9443",
		State:          TunnelStateActive,
	}); !isUniqueViolation(err) {
		t.Fatalf("expected tunnel unique violation, got %v", err)
	}

	if _, err := store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID: org.ID,
		AccountID:      uuid.New(),
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

	orgID := uuid.New()
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
		Role:           AccountRoleAdmin,
		Status:         AccountStatusActive,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}

	description := "build agent"
	host := "agent-01"
	env, err := store.Environments.Create(ctx, store.DB(), Environment{
		OrganizationID: org.ID,
		AccountID:      acct.ID,
		Description:    &description,
		Host:           &host,
		ZitiIdentityID: "ziti-identity-01",
		State:          EnvironmentStateEnabled,
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
