package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type Queryer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row
}

type Store struct {
	db                   *sqlx.DB
	Organizations        *OrganizationsRepository
	Accounts             *AccountsRepository
	Environments         *EnvironmentsRepository
	Tunnels              *TunnelsRepository
	TunnelGrants         *TunnelGrantsRepository
	TunnelAttachments    *TunnelAttachmentsRepository
	TunnelServes         *TunnelServesRepository
	Workgroups           *WorkgroupsRepository
	WorkgroupInvitations *WorkgroupInvitationsRepository
	WorkgroupMemberships *WorkgroupMembershipsRepository
	Advertisements       *AdvertisementsRepository
}

func Open(ctx context.Context, cfg Config) (*Store, error) {
	if cfg.DSN == "" {
		return nil, errors.New("persistence: DSN is required")
	}

	db, err := sqlx.Open("pgx", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("persistence: open database: %w", err)
	}

	if cfg.MaxOpenConns > 0 {
		db.SetMaxOpenConns(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		db.SetMaxIdleConns(cfg.MaxIdleConns)
	}
	if cfg.ConnMaxLifetime > 0 {
		db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	}
	db.SetConnMaxIdleTime(5 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("persistence: ping database: %w", err)
	}

	store := &Store{db: db}
	store.Organizations = &OrganizationsRepository{}
	store.Accounts = &AccountsRepository{}
	store.Environments = &EnvironmentsRepository{}
	store.Tunnels = &TunnelsRepository{}
	store.TunnelGrants = &TunnelGrantsRepository{}
	store.TunnelAttachments = &TunnelAttachmentsRepository{}
	store.TunnelServes = &TunnelServesRepository{}
	store.Workgroups = &WorkgroupsRepository{}
	store.WorkgroupInvitations = &WorkgroupInvitationsRepository{}
	store.WorkgroupMemberships = &WorkgroupMembershipsRepository{}
	store.Advertisements = &AdvertisementsRepository{}

	return store, nil
}

func (s *Store) DB() *sqlx.DB {
	return s.db
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) WithTx(ctx context.Context, fn func(Queryer) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("persistence: begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("persistence: commit transaction: %w", err)
	}
	return nil
}
