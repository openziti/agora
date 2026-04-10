package persistence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type OrganizationsRepository struct{}

func (r *OrganizationsRepository) Create(ctx context.Context, db Queryer, org Organization) (*Organization, error) {
	if org.ID == "" {
		org.ID = NewResourceID(PrefixOrganization)
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

func (r *OrganizationsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Organization, error) {
	const query = `
select id, name, created_at, updated_at
from organizations
where id = $1`

	var org Organization
	if err := db.GetContext(ctx, &org, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get organization: %w", err)
	}
	return &org, nil
}

func (r *OrganizationsRepository) List(ctx context.Context, db Queryer) ([]Organization, error) {
	const query = `
select id, name, created_at, updated_at
from organizations
order by name asc`

	var organizations []Organization
	if err := db.SelectContext(ctx, &organizations, query); err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	return organizations, nil
}

func (r *OrganizationsRepository) Delete(ctx context.Context, db Queryer, id string) error {
	const query = `delete from organizations where id = $1`

	result, err := db.ExecContext(ctx, query, id)
	if err != nil {
		if isForeignKeyConstraint(err) {
			return ErrConflict
		}
		return fmt.Errorf("delete organization: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete organization rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type AccountsRepository struct{}

func (r *AccountsRepository) Create(ctx context.Context, db Queryer, acct Account) (*Account, error) {
	if acct.ID == "" {
		acct.ID = NewResourceID(PrefixAccount)
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
	acct.Email = strings.ToLower(strings.TrimSpace(acct.Email))

	const query = `
insert into accounts (
	id, organization_id, email, display_name, password_salt, password_hash, account_token, role, status, created_at, updated_at
) values (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
)
returning id, organization_id, email, display_name, password_salt, password_hash, account_token, role, status, created_at, updated_at`

	var created Account
	if err := db.GetContext(
		ctx,
		&created,
		query,
		acct.ID,
		acct.OrganizationID,
		acct.Email,
		acct.DisplayName,
		acct.PasswordSalt,
		acct.PasswordHash,
		acct.AccountToken,
		acct.Role,
		acct.Status,
		acct.CreatedAt,
		acct.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create account: %w", err)
	}
	return &created, nil
}

func (r *AccountsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Account, error) {
	const query = `
select id, organization_id, email, display_name, password_salt, password_hash, account_token, role, status, created_at, updated_at
from accounts
where id = $1`

	var acct Account
	if err := db.GetContext(ctx, &acct, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get account: %w", err)
	}
	return &acct, nil
}

func (r *AccountsRepository) FindByEmail(ctx context.Context, db Queryer, email string) (*Account, error) {
	const query = `
select id, organization_id, email, display_name, password_salt, password_hash, account_token, role, status, created_at, updated_at
from accounts
where lower(email) = lower($1)`

	var acct Account
	if err := db.GetContext(ctx, &acct, query, strings.TrimSpace(email)); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find account by email: %w", err)
	}
	return &acct, nil
}

func (r *AccountsRepository) FindByToken(ctx context.Context, db Queryer, token string) (*Account, error) {
	const query = `
select id, organization_id, email, display_name, password_salt, password_hash, account_token, role, status, created_at, updated_at
from accounts
where account_token = $1`

	var acct Account
	if err := db.GetContext(ctx, &acct, query, token); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("find account by token: %w", err)
	}
	return &acct, nil
}

func (r *AccountsRepository) ListByOrganization(ctx context.Context, db Queryer, organizationID string) ([]Account, error) {
	return r.List(ctx, db, &organizationID)
}

func (r *AccountsRepository) List(ctx context.Context, db Queryer, organizationID *string) ([]Account, error) {
	query := `
select id, organization_id, email, display_name, password_salt, password_hash, account_token, role, status, created_at, updated_at
from accounts`
	args := []any{}
	if organizationID != nil {
		query += `
where organization_id = $1`
		args = append(args, *organizationID)
	}
	query += `
order by organization_id asc, email asc`

	var accounts []Account
	if err := db.SelectContext(ctx, &accounts, query, args...); err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	return accounts, nil
}

func (r *AccountsRepository) UpdatePassword(ctx context.Context, db Queryer, accountID string, salt, hash string) error {
	const query = `
update accounts
set password_salt = $2, password_hash = $3, updated_at = $4
where id = $1`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, accountID, salt, hash, updatedAt)
	if err != nil {
		return fmt.Errorf("update account password: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update account password rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AccountsRepository) RotateToken(ctx context.Context, db Queryer, accountID string, token string) error {
	const query = `
update accounts
set account_token = $2, updated_at = $3
where id = $1`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, accountID, token, updatedAt)
	if err != nil {
		return fmt.Errorf("rotate account token: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rotate account token rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *AccountsRepository) Delete(ctx context.Context, db Queryer, organizationID, accountID string) error {
	const query = `delete from accounts where organization_id = $1 and id = $2`

	result, err := db.ExecContext(ctx, query, organizationID, accountID)
	if err != nil {
		if isForeignKeyConstraint(err) {
			return ErrConflict
		}
		return fmt.Errorf("delete account: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete account rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

type EnvironmentsRepository struct{}

func (r *EnvironmentsRepository) Create(ctx context.Context, db Queryer, env Environment) (*Environment, error) {
	if env.ID == "" {
		env.ID = NewResourceID(PrefixEnvironment)
	}
	now := time.Now().UTC()
	env.CreatedAt = now
	env.UpdatedAt = now
	if env.State == "" {
		env.State = EnvironmentStateEnabled
	}

	const query = `
insert into environments (
	id, organization_id, account_id, description, host, ziti_identity_id, edge_router_policy_id, state, deleted, last_seen_at, created_at, updated_at
) values (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
returning id, organization_id, account_id, description, host, ziti_identity_id, edge_router_policy_id, state, deleted, last_seen_at, created_at, updated_at`

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
		env.EdgeRouterPolicyID,
		env.State,
		env.Deleted,
		env.LastSeenAt,
		env.CreatedAt,
		env.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create environment: %w", err)
	}
	return &created, nil
}

func (r *EnvironmentsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Environment, error) {
	const query = `
select id, organization_id, account_id, description, host, ziti_identity_id, edge_router_policy_id, state, deleted, last_seen_at, created_at, updated_at
from environments
where id = $1 and not deleted`

	var env Environment
	if err := db.GetContext(ctx, &env, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get environment: %w", err)
	}
	return &env, nil
}

func (r *EnvironmentsRepository) ListByOrganization(ctx context.Context, db Queryer, organizationID string) ([]Environment, error) {
	const query = `
select id, organization_id, account_id, description, host, ziti_identity_id, edge_router_policy_id, state, deleted, last_seen_at, created_at, updated_at
from environments
where organization_id = $1 and not deleted
order by created_at asc`

	var environments []Environment
	if err := db.SelectContext(ctx, &environments, query, organizationID); err != nil {
		return nil, fmt.Errorf("list environments: %w", err)
	}
	return environments, nil
}

func (r *EnvironmentsRepository) Delete(ctx context.Context, db Queryer, id string, organizationID string) error {
	const query = `
update environments
set state = $3, deleted = true, updated_at = $4
where id = $1 and organization_id = $2 and not deleted`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, organizationID, EnvironmentStateDisabled, updatedAt)
	if err != nil {
		return fmt.Errorf("delete environment: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete environment rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *EnvironmentsRepository) ListByAccount(ctx context.Context, db Queryer, accountID string) ([]Environment, error) {
	const query = `
select id, organization_id, account_id, description, host, ziti_identity_id, edge_router_policy_id, state, deleted, last_seen_at, created_at, updated_at
from environments
where account_id = $1 and not deleted
order by created_at asc`

	var environments []Environment
	if err := db.SelectContext(ctx, &environments, query, accountID); err != nil {
		return nil, fmt.Errorf("list account environments: %w", err)
	}
	return environments, nil
}

func (r *EnvironmentsRepository) UpdateLastSeen(ctx context.Context, db Queryer, id string, lastSeenAt time.Time) error {
	const query = `
update environments
set last_seen_at = $2, updated_at = $3
where id = $1 and not deleted`

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
	if tunnel.ID == "" {
		tunnel.ID = NewResourceID(PrefixTunnel)
	}
	now := time.Now().UTC()
	tunnel.CreatedAt = now
	tunnel.UpdatedAt = now
	if tunnel.State == "" {
		tunnel.State = TunnelStateActive
	}

	const query = `
insert into tunnels (
	id, organization_id, environment_id, name, backend_address, ziti_service_id, state, deleted, created_at, updated_at
) values (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
)
returning id, organization_id, environment_id, name, backend_address, ziti_service_id, state, deleted, created_at, updated_at`

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
		tunnel.Deleted,
		tunnel.CreatedAt,
		tunnel.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create tunnel: %w", err)
	}
	return &created, nil
}

func (r *TunnelsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Tunnel, error) {
	const query = `
select id, organization_id, environment_id, name, backend_address, ziti_service_id, state, deleted, created_at, updated_at
from tunnels
where id = $1 and not deleted`

	var tunnel Tunnel
	if err := db.GetContext(ctx, &tunnel, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get tunnel: %w", err)
	}
	return &tunnel, nil
}

func (r *TunnelsRepository) ListByOrganization(ctx context.Context, db Queryer, organizationID string) ([]Tunnel, error) {
	const query = `
select id, organization_id, environment_id, name, backend_address, ziti_service_id, state, deleted, created_at, updated_at
from tunnels
where organization_id = $1 and not deleted
order by name asc`

	var tunnels []Tunnel
	if err := db.SelectContext(ctx, &tunnels, query, organizationID); err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	return tunnels, nil
}

func isForeignKeyConstraint(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}

func (r *TunnelsRepository) Delete(ctx context.Context, db Queryer, id string, organizationID string) error {
	const query = `
update tunnels
set state = $3, deleted = true, updated_at = $4
where id = $1 and organization_id = $2 and not deleted`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, organizationID, TunnelStateDisabled, updatedAt)
	if err != nil {
		return fmt.Errorf("delete tunnel: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete tunnel rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *TunnelsRepository) ListByEnvironment(ctx context.Context, db Queryer, environmentID, organizationID string) ([]Tunnel, error) {
	const query = `
select id, organization_id, environment_id, name, backend_address, ziti_service_id, state, deleted, created_at, updated_at
from tunnels
where environment_id = $1 and organization_id = $2 and not deleted
order by name asc`

	var tunnels []Tunnel
	if err := db.SelectContext(ctx, &tunnels, query, environmentID, organizationID); err != nil {
		return nil, fmt.Errorf("list environment tunnels: %w", err)
	}
	return tunnels, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}
