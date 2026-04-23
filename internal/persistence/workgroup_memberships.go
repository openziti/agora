package persistence

import (
	"context"
	"fmt"
	"time"
)

type WorkgroupMembershipsRepository struct{}

func (r *WorkgroupMembershipsRepository) Create(ctx context.Context, db Queryer, membership WorkgroupMembership) (*WorkgroupMembership, error) {
	if membership.ID == "" {
		membership.ID = NewResourceID(PrefixWorkgroupMembership)
	}
	now := time.Now().UTC()
	membership.CreatedAt = now
	membership.UpdatedAt = now
	if membership.JoinedAt.IsZero() {
		membership.JoinedAt = now
	}
	if membership.Role == "" {
		membership.Role = WorkgroupMembershipRoleMember
	}

	const query = `
insert into workgroup_memberships (
    id, workgroup_id, organization_id, account_id, role, joined_at, deleted, created_at, updated_at
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
returning id, workgroup_id, organization_id, account_id, role, joined_at, deleted, created_at, updated_at`

	var created WorkgroupMembership
	if err := db.GetContext(
		ctx,
		&created,
		query,
		membership.ID,
		membership.WorkgroupID,
		membership.OrganizationID,
		membership.AccountID,
		membership.Role,
		membership.JoinedAt,
		membership.Deleted,
		membership.CreatedAt,
		membership.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create workgroup membership: %w", err)
	}
	return &created, nil
}

func (r *WorkgroupMembershipsRepository) GetByID(ctx context.Context, db Queryer, id string) (*WorkgroupMembership, error) {
	const query = `
select id, workgroup_id, organization_id, account_id, role, joined_at, deleted, created_at, updated_at
from workgroup_memberships
where id = $1 and not deleted`

	var membership WorkgroupMembership
	if err := db.GetContext(ctx, &membership, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workgroup membership: %w", err)
	}
	return &membership, nil
}

func (r *WorkgroupMembershipsRepository) GetByWorkgroupAndAccount(ctx context.Context, db Queryer, workgroupID, accountID string) (*WorkgroupMembership, error) {
	const query = `
select id, workgroup_id, organization_id, account_id, role, joined_at, deleted, created_at, updated_at
from workgroup_memberships
where workgroup_id = $1 and account_id = $2 and not deleted`

	var membership WorkgroupMembership
	if err := db.GetContext(ctx, &membership, query, workgroupID, accountID); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workgroup membership by workgroup+account: %w", err)
	}
	return &membership, nil
}

func (r *WorkgroupMembershipsRepository) ListByWorkgroup(ctx context.Context, db Queryer, workgroupID string) ([]WorkgroupMembership, error) {
	const query = `
select id, workgroup_id, organization_id, account_id, role, joined_at, deleted, created_at, updated_at
from workgroup_memberships
where workgroup_id = $1 and not deleted
order by joined_at asc`

	var memberships []WorkgroupMembership
	if err := db.SelectContext(ctx, &memberships, query, workgroupID); err != nil {
		return nil, fmt.Errorf("list workgroup memberships: %w", err)
	}
	return memberships, nil
}

func (r *WorkgroupMembershipsRepository) ListByAccount(ctx context.Context, db Queryer, accountID string) ([]WorkgroupMembership, error) {
	const query = `
select id, workgroup_id, organization_id, account_id, role, joined_at, deleted, created_at, updated_at
from workgroup_memberships
where account_id = $1 and not deleted
order by joined_at asc`

	var memberships []WorkgroupMembership
	if err := db.SelectContext(ctx, &memberships, query, accountID); err != nil {
		return nil, fmt.Errorf("list workgroup memberships by account: %w", err)
	}
	return memberships, nil
}

// CountAdmins returns the number of non-deleted admin memberships on
// the given workgroup. Used by the at-least-one-admin invariant: a
// role demotion or membership removal is rejected if it would drive
// this count to zero.
func (r *WorkgroupMembershipsRepository) CountAdmins(ctx context.Context, db Queryer, workgroupID string) (int, error) {
	const query = `
select count(*)
from workgroup_memberships
where workgroup_id = $1 and role = 'admin' and not deleted`

	var count int
	if err := db.GetContext(ctx, &count, query, workgroupID); err != nil {
		return 0, fmt.Errorf("count workgroup admins: %w", err)
	}
	return count, nil
}

func (r *WorkgroupMembershipsRepository) UpdateRole(ctx context.Context, db Queryer, id string, role WorkgroupMembershipRole) error {
	const query = `
update workgroup_memberships
set role = $2, updated_at = $3
where id = $1 and not deleted`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, role, updatedAt)
	if err != nil {
		return fmt.Errorf("update workgroup membership role: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update workgroup membership role rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *WorkgroupMembershipsRepository) MarkDeleted(ctx context.Context, db Queryer, id string) error {
	const query = `
update workgroup_memberships
set deleted = true, updated_at = $2
where id = $1 and not deleted`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, updatedAt)
	if err != nil {
		return fmt.Errorf("mark workgroup membership deleted: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark workgroup membership deleted rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDeleteAllForWorkgroup soft-deletes every membership on the
// given workgroup. Used by the workgroup delete cascade.
func (r *WorkgroupMembershipsRepository) SoftDeleteAllForWorkgroup(ctx context.Context, db Queryer, workgroupID string) error {
	const query = `
update workgroup_memberships
set deleted = true, updated_at = $2
where workgroup_id = $1 and not deleted`

	updatedAt := time.Now().UTC()
	if _, err := db.ExecContext(ctx, query, workgroupID, updatedAt); err != nil {
		return fmt.Errorf("soft-delete all workgroup memberships: %w", err)
	}
	return nil
}
