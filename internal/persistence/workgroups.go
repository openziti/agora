package persistence

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type WorkgroupsRepository struct{}

func (r *WorkgroupsRepository) Create(ctx context.Context, db Queryer, wg Workgroup) (*Workgroup, error) {
	if wg.ID == "" {
		wg.ID = NewResourceID(PrefixWorkgroup)
	}
	now := time.Now().UTC()
	wg.CreatedAt = now
	wg.UpdatedAt = now
	if wg.State == "" {
		wg.State = WorkgroupStatePending
	}

	const query = `
insert into workgroups (
    id, owner_organization_id, name, description, scope, state, deleted, created_at, updated_at
) values (
    $1, $2, $3, $4, $5, $6, $7, $8, $9
)
returning id, owner_organization_id, name, description, scope, state, deleted, created_at, updated_at`

	var created Workgroup
	if err := db.GetContext(
		ctx,
		&created,
		query,
		wg.ID,
		wg.OwnerOrganizationID,
		strings.TrimSpace(wg.Name),
		wg.Description,
		wg.Scope,
		wg.State,
		wg.Deleted,
		wg.CreatedAt,
		wg.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("create workgroup: %w", err)
	}
	return &created, nil
}

func (r *WorkgroupsRepository) GetByID(ctx context.Context, db Queryer, id string) (*Workgroup, error) {
	const query = `
select id, owner_organization_id, name, description, scope, state, deleted, created_at, updated_at
from workgroups
where id = $1 and not deleted`

	var wg Workgroup
	if err := db.GetContext(ctx, &wg, query, id); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workgroup: %w", err)
	}
	return &wg, nil
}

func (r *WorkgroupsRepository) GetByNameInOrg(ctx context.Context, db Queryer, ownerOrganizationID, name string) (*Workgroup, error) {
	const query = `
select id, owner_organization_id, name, description, scope, state, deleted, created_at, updated_at
from workgroups
where owner_organization_id = $1 and lower(name) = lower($2) and not deleted and state <> 'declined'`

	var wg Workgroup
	if err := db.GetContext(ctx, &wg, query, ownerOrganizationID, strings.TrimSpace(name)); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get workgroup by name: %w", err)
	}
	return &wg, nil
}

// ListAccessibleByAccount returns workgroups the given account can see
// through the account-token surface: any workgroup the account is a
// non-deleted member of.
func (r *WorkgroupsRepository) ListAccessibleByAccount(ctx context.Context, db Queryer, accountID string) ([]Workgroup, error) {
	const query = `
select distinct
    w.id, w.owner_organization_id, w.name, w.description, w.scope, w.state, w.deleted, w.created_at, w.updated_at
from workgroups w
join workgroup_memberships m on m.workgroup_id = w.id and not m.deleted
where m.account_id = $1 and not w.deleted
order by w.updated_at desc, w.name asc`

	var workgroups []Workgroup
	if err := db.SelectContext(ctx, &workgroups, query, accountID); err != nil {
		return nil, fmt.Errorf("list workgroups accessible to account: %w", err)
	}
	return workgroups, nil
}

// ListForAdmin returns workgroups across all organizations subject to
// optional filters. ownerOrganizationID, invitedOrganizationID, and
// state are each applied if non-empty.
func (r *WorkgroupsRepository) ListForAdmin(ctx context.Context, db Queryer, ownerOrganizationID, invitedOrganizationID string, state WorkgroupState) ([]Workgroup, error) {
	clauses := []string{"not w.deleted"}
	args := []any{}
	if ownerOrganizationID != "" {
		args = append(args, ownerOrganizationID)
		clauses = append(clauses, fmt.Sprintf("w.owner_organization_id = $%d", len(args)))
	}
	if state != "" {
		args = append(args, state)
		clauses = append(clauses, fmt.Sprintf("w.state = $%d", len(args)))
	}
	join := ""
	if invitedOrganizationID != "" {
		args = append(args, invitedOrganizationID)
		join = fmt.Sprintf("join workgroup_invitations i on i.workgroup_id = w.id and i.organization_id = $%d", len(args))
	}

	query := fmt.Sprintf(`
select distinct
    w.id, w.owner_organization_id, w.name, w.description, w.scope, w.state, w.deleted, w.created_at, w.updated_at
from workgroups w
%s
where %s
order by w.updated_at desc`, join, strings.Join(clauses, " and "))

	var workgroups []Workgroup
	if err := db.SelectContext(ctx, &workgroups, query, args...); err != nil {
		return nil, fmt.Errorf("list workgroups for admin: %w", err)
	}
	return workgroups, nil
}

func (r *WorkgroupsRepository) UpdateState(ctx context.Context, db Queryer, id string, state WorkgroupState) error {
	const query = `
update workgroups
set state = $2, updated_at = $3
where id = $1 and not deleted`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, state, updatedAt)
	if err != nil {
		return fmt.Errorf("update workgroup state: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update workgroup state rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *WorkgroupsRepository) MarkDeleted(ctx context.Context, db Queryer, id string) error {
	const query = `
update workgroups
set deleted = true, updated_at = $2
where id = $1 and not deleted`

	updatedAt := time.Now().UTC()
	result, err := db.ExecContext(ctx, query, id, updatedAt)
	if err != nil {
		return fmt.Errorf("mark workgroup deleted: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark workgroup deleted rows affected: %w", err)
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
