package persistence

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type OrganizationsRepository struct{}

func (r *OrganizationsRepository) Create(ctx context.Context, db Queryer, org Organization) (*Organization, error) {
	if org.ID == uuid.Nil {
		org.ID = uuid.New()
	}
	now := time.Now().UTC()
	org.CreatedAt = now
	org.UpdatedAt = now

	const query = `
insert into organizations (id, name, created_at, updated_at)
values ($1, $2, $3, $4)
returning id, name, created_at, updated_at`

	var created Organization
	if err := db.GetContext(ctx, &created, query, org.ID, org.Name, org.CreatedAt, org.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create organization: %w", err)
	}
	return &created, nil
}

func (r *OrganizationsRepository) GetByID(ctx context.Context, db Queryer, id uuid.UUID) (*Organization, error) {
	const query = `
select id, name, created_at, updated_at
from organizations
where id = $1`

	var org Organization
	if err := db.GetContext(ctx, &org, query, id); err != nil {
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return &org, nil
}

type AccountsRepository struct{}

func (r *AccountsRepository) Create(ctx context.Context, db Queryer, acct Account) (*Account, error) {
	if acct.ID == uuid.Nil {
		acct.ID = uuid.New()
	}
	now := time.Now().UTC()
	acct.CreatedAt = now
	acct.UpdatedAt = now
	if acct.Role == "" {
		acct.Role = AccountRoleMember
	}
	if acct.Status == "" {
		acct.Status = AccountStatusActive
	}

	const query = `
insert into accounts (
	id, organization_id, email, display_name, role, status, created_at, updated_at
) values (
	$1, $2, $3, $4, $5, $6, $7, $8
)
returning id, organization_id, email, display_name, role, status, created_at, updated_at`

	var created Account
	if err := db.GetContext(
		ctx,
		&created,
		query,
		acct.ID,
		acct.OrganizationID,
		acct.Email,
		acct.DisplayName,
		acct.Role,
		acct.Status,
		acct.CreatedAt,
		acct.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return &created, nil
}

func (r *AccountsRepository) GetByID(ctx context.Context, db Queryer, id uuid.UUID) (*Account, error) {
	const query = `
select id, organization_id, email, display_name, role, status, created_at, updated_at
from accounts
where id = $1`

	var acct Account
	if err := db.GetContext(ctx, &acct, query, id); err != nil {
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &acct, nil
}

type EnvironmentsRepository struct{}

func (r *EnvironmentsRepository) Create(ctx context.Context, db Queryer, env Environment) (*Environment, error) {
	if env.ID == uuid.Nil {
		env.ID = uuid.New()
	}
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now
	if env.State == "" {
		env.State = EnvironmentStateEnabled
	}

	const query = `
insert into environments (
	id, organization_id, account_id, description, host, ziti_identity_id, state, last_seen_at, created_at, updated_at
) values (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
returning id, organization_id, account_id, description, host, ziti_identity_id, state, last_seen_at, created_at, updated_at`

	var created Environment
	if err := db.GetContext(
		ctx,
		&created,
		query,
		env.ID,
		env.OrganizationID,
		env.AccountID,
		env.Description,
		env.Host,
		env.ZitiIdentityID,
		env.State,
		env.LastSeenAt,
		env.CreatedAt,
		env.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create environment: %w", err)
	}
	return &created, nil
}

func (r *EnvironmentsRepository) GetByID(ctx context.Context, db Queryer, id uuid.UUID) (*Environment, error) {
	const query = `
select id, organization_id, account_id, description, host, ziti_identity_id, state, last_seen_at, created_at, updated_at
from environments
where id = $1`

	var env Environment
	if err := db.GetContext(ctx, &env, query, id); err != nil {
		return nil, fmt.Errorf("get environment: %w", err)
	}
	return &env, nil
}

func (r *EnvironmentsRepository) UpdateLastSeen(ctx context.Context, db Queryer, id uuid.UUID, lastSeenAt time.Time) error {
	const query = `
update environments
set last_seen_at = $2, updated_at = $3
where id = $1`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, lastSeenAt.UTC(), updatedAt)
	if err != nil {
		return fmt.Errorf("update environment last seen: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update environment last seen rows affected: %w", err)
	}
	if rows == 0 {
		return errors.New("update environment last seen: not found")
	}
	return nil
}

type TunnelsRepository struct{}

func (r *TunnelsRepository) Create(ctx context.Context, db Queryer, tunnel Tunnel) (*Tunnel, error) {
	if tunnel.ID == uuid.Nil {
		tunnel.ID = uuid.New()
	}
	now := time.Now().UTC()
	tunnel.CreatedAt = now
	tunnel.UpdatedAt = now
	if tunnel.State == "" {
		tunnel.State = TunnelStateActive
	}

	const query = `
insert into tunnels (
	id, organization_id, environment_id, name, backend_address, ziti_service_id, state, created_at, updated_at
) values (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)
returning id, organization_id, environment_id, name, backend_address, ziti_service_id, state, created_at, updated_at`

	var created Tunnel
	if err := db.GetContext(
		ctx,
		&created,
		query,
		tunnel.ID,
		tunnel.OrganizationID,
		tunnel.EnvironmentID,
		tunnel.Name,
		tunnel.BackendAddress,
		tunnel.ZitiServiceID,
		tunnel.State,
		tunnel.CreatedAt,
		tunnel.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create tunnel: %w", err)
	}
	return &created, nil
}

func (r *TunnelsRepository) GetByID(ctx context.Context, db Queryer, id uuid.UUID) (*Tunnel, error) {
	const query = `
select id, organization_id, environment_id, name, backend_address, ziti_service_id, state, created_at, updated_at
from tunnels
where id = $1`

	var tunnel Tunnel
	if err := db.GetContext(ctx, &tunnel, query, id); err != nil {
		return nil, fmt.Errorf("get tunnel: %w", err)
	}
	return &tunnel, nil
}
