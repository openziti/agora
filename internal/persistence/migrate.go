package persistence

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5/pgconn"
	migrate "github.com/rubenv/sql-migrate"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

type MigrationStatus struct {
	ID      string
	Applied bool
}

var (
	migrationTableName    = "migrations"
	ErrNoMigrationsTable  = errors.New("persistence: migrations table missing")
	ErrSchemaBehindBinary = errors.New("persistence: schema behind binary")
)

func MigrateUp(ctx context.Context, store *Store) (int, error) {
	return execMigrations(ctx, store, migrate.Up, 0)
}

func MigrateDown(ctx context.Context, store *Store, steps int) (int, error) {
	if steps < 1 {
		return 0, errors.New("persistence: down migrations require steps >= 1")
	}
	return execMigrations(ctx, store, migrate.Down, steps)
}

func MigrationStatusList(ctx context.Context, store *Store) ([]MigrationStatus, error) {
	source := migrationSource()
	migrations, err := source.FindMigrations()
	if err != nil {
		return nil, fmt.Errorf("persistence: list embedded migrations: %w", err)
	}

	exists, err := hasMigrationsTable(ctx, store)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNoMigrationsTable
	}

	migrate.SetTable(migrationTableName)
	records, err := migrate.GetMigrationRecords(store.db.DB, "postgres")
	if err != nil {
		if isUndefinedTable(err) {
			return nil, ErrNoMigrationsTable
		}
		return nil, fmt.Errorf("persistence: get migration records: %w", err)
	}

	applied := map[string]struct{}{}
	for _, record := range records {
		applied[record.Id] = struct{}{}
	}

	statuses := make([]MigrationStatus, 0, len(migrations))
	for _, migration := range migrations {
		_, ok := applied[migration.Id]
		statuses = append(statuses, MigrationStatus{ID: migration.Id, Applied: ok})
	}
	return statuses, nil
}

func CheckSchemaCompatibility(ctx context.Context, store *Store) error {
	statuses, err := MigrationStatusList(ctx, store)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		if !status.Applied {
			return fmt.Errorf("%w: unapplied migration %s", ErrSchemaBehindBinary, status.ID)
		}
	}
	return nil
}

func execMigrations(ctx context.Context, store *Store, direction migrate.MigrationDirection, max int) (int, error) {
	_ = ctx

	source := migrationSource()
	migrate.SetTable(migrationTableName)
	n, err := migrate.ExecMax(store.db.DB, "postgres", source, direction, max)
	if err != nil {
		return 0, fmt.Errorf("persistence: run migrations: %w", err)
	}
	return n, nil
}

func migrationSource() *migrate.EmbedFileSystemMigrationSource {
	return &migrate.EmbedFileSystemMigrationSource{
		FileSystem: migrationFS,
		Root:       "migrations",
	}
}

func AvailableMigrationIDs() ([]string, error) {
	source := migrationSource()
	migrations, err := source.FindMigrations()
	if err != nil {
		return nil, fmt.Errorf("persistence: list embedded migrations: %w", err)
	}
	ids := make([]string, 0, len(migrations))
	for _, migration := range migrations {
		ids = append(ids, migration.Id)
	}
	sort.Strings(ids)
	return ids, nil
}

func isUndefinedTable(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "42P01"
}

func hasMigrationsTable(ctx context.Context, store *Store) (bool, error) {
	var name *string
	if err := store.db.GetContext(ctx, &name, "select to_regclass($1)", migrationTableName); err != nil {
		return false, fmt.Errorf("persistence: check migrations table: %w", err)
	}
	return name != nil, nil
}
